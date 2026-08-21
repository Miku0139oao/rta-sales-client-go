package desktop

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	rtasales "github.com/Miku0139oao/rta-sales-client-go/rtasales"
	"github.com/Miku0139oao/rta-sales-client-go/securestore"
	"github.com/Miku0139oao/rta-sales-client-go/xlsxfill"
)

const (
	salesAnalysisProgressEventName = "rta:sales-analysis-progress"
	salesAnalysisUpdateEventName   = "rta:sales-analysis-update"
)

var salesAnalysisRateLimitRetryDelays = [...]time.Duration{time.Second, 3 * time.Second}

// ListSalesAnalysisStores loads authorized stores from one profile, or from
// every enabled profile when ProfileID is empty. Overlapping stores keep the
// earlier profile in account priority order.
func (a *App) ListSalesAnalysisStores(request ProfileIDRequest) ([]SalesAnalysisStore, error) {
	operationID, err := newUUID()
	if err != nil {
		return nil, err
	}
	ctx, finish, err := a.beginSalesAnalysisOperation(operationID)
	if err != nil {
		return nil, err
	}
	defer finish()

	routes, err := a.salesAnalysisRoutes(ctx, request.ProfileID, request.SimulateStoreCount)
	if err != nil {
		return nil, err
	}
	result := make([]SalesAnalysisStore, 0, len(routes))
	for _, route := range routes {
		businessID := strings.TrimSpace(route.store.BusinessID)
		if businessID == "" {
			continue
		}
		result = append(result, SalesAnalysisStore{
			BusinessID: businessID,
			Label:      strings.TrimSpace(route.store.Label),
			ProfileID:  route.profileID,
			Profile:    route.profile,
		})
	}
	sort.Slice(result, func(left, right int) bool { return result[left].BusinessID < result[right].BusinessID })
	return result, nil
}

// RunSalesAnalysis returns current-period Article View first so a 16-store
// account can show the overview quickly. Trend View is one all-stores query
// because RTA already aggregates it. Other periods and that trend run in the
// background; each store's independent article report stays a separate query.
func (a *App) RunSalesAnalysis(request SalesAnalysisRequest) (SalesAnalysisResult, error) {
	started := time.Now()
	periods, err := normalizeSalesAnalysisPeriods(request)
	if err != nil {
		return SalesAnalysisResult{}, err
	}
	storeIDs := normalizeSalesAnalysisStoreIDs(request.StoreIDs)
	if len(storeIDs) == 0 {
		return SalesAnalysisResult{}, errors.New("at least one storeId is required")
	}
	concurrency := request.Concurrency
	if concurrency == 0 {
		concurrency = xlsxfill.DefaultConcurrency
	}
	if concurrency < 1 || concurrency > xlsxfill.MaximumConcurrency {
		return SalesAnalysisResult{}, fmt.Errorf("analysis concurrency must be between 1 and %d", xlsxfill.MaximumConcurrency)
	}
	operationID, err := newUUID()
	if err != nil {
		return SalesAnalysisResult{}, err
	}
	ctx, finish, err := a.beginSalesAnalysisOperation(operationID)
	if err != nil {
		return SalesAnalysisResult{}, err
	}
	released := false
	defer func() {
		if !released {
			finish()
		}
	}()

	selected, err := a.selectSalesAnalysisStores(ctx, request.ProfileID, storeIDs, request.SimulateStoreCount)
	if err != nil {
		return SalesAnalysisResult{}, err
	}
	primaryIndex := primarySalesAnalysisPeriodIndex(periods)
	primaryJobs := articleJobsForPeriod(primaryIndex, len(selected))
	followJobs := followOnSalesAnalysisJobs(periods, primaryIndex, len(selected))
	totalTasks := len(primaryJobs) + len(followJobs)

	a.events.Emit(a.appContext(), salesAnalysisProgressEventName, SalesAnalysisProgress{
		OperationID: operationID, Current: 0, Total: totalTasks, Status: "running",
	})
	primaryOutcomes, completed, err := a.runSalesAnalysisJobs(ctx, operationID, selected, periods, primaryJobs, 0, totalTasks, concurrency)
	if err != nil {
		return SalesAnalysisResult{}, err
	}

	analysis, packed := assembleSalesAnalysisArticles(operationID, selected, []normalizedSalesAnalysisPeriod{periods[primaryIndex]}, [][]storeOutcome{primaryOutcomes[primaryIndex]})
	analysis.Pending = len(followJobs) > 0
	analysis.Complete = !analysis.Pending && len(analysis.Issues) == 0
	analysis.QueryDurationMS = time.Since(started).Milliseconds()
	remembered := a.rememberSalesAnalysis(analysis, packed)
	if len(followJobs) == 0 {
		released = true
		finish()
		return remembered, nil
	}

	released = true
	go a.supplementSalesAnalysis(ctx, finish, started, operationID, selected, periods, primaryIndex, followJobs, completed, totalTasks, concurrency)
	return remembered, nil
}

type selectedStore struct {
	route analysisStoreRoute
	query accountClient
}

type storeOutcome struct {
	result *rtasales.SalesResult
	err    error
}

type analysisJob struct {
	kind        string
	periodIndex int
	storeIndex  int
}

type normalizedSalesAnalysisPeriod struct {
	key          string
	label        string
	from         time.Time
	to           time.Time
	includeTrend bool
}

func (a *App) selectSalesAnalysisStores(ctx context.Context, profileID string, storeIDs []string, simulateStoreCount int) ([]selectedStore, error) {
	routes, err := a.salesAnalysisRoutes(ctx, profileID, simulateStoreCount)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]analysisStoreRoute, len(routes))
	for _, route := range routes {
		byID[strings.TrimSpace(route.store.BusinessID)] = route
	}
	selected := make([]selectedStore, 0, len(storeIDs))
	for _, storeID := range storeIDs {
		route, ok := byID[storeID]
		if !ok {
			return nil, fmt.Errorf("store %q is not authorized for the selected profiles", storeID)
		}
		selected = append(selected, selectedStore{
			route: route,
			query: maybeSimulateClient(route.client, simulateStoreCount),
		})
	}
	return selected, nil
}

func primarySalesAnalysisPeriodIndex(periods []normalizedSalesAnalysisPeriod) int {
	for index, period := range periods {
		if period.key == "current" {
			return index
		}
	}
	return 0
}

func articleJobsForPeriod(periodIndex, storeCount int) []analysisJob {
	jobs := make([]analysisJob, 0, storeCount)
	for storeIndex := 0; storeIndex < storeCount; storeIndex++ {
		jobs = append(jobs, analysisJob{kind: "article", periodIndex: periodIndex, storeIndex: storeIndex})
	}
	return jobs
}

func followOnSalesAnalysisJobs(periods []normalizedSalesAnalysisPeriod, primaryIndex, storeCount int) []analysisJob {
	jobs := make([]analysisJob, 0)
	if periods[primaryIndex].includeTrend {
		jobs = append(jobs, analysisJob{kind: "trend", periodIndex: primaryIndex, storeIndex: -1})
	}
	for periodIndex := range periods {
		if periodIndex == primaryIndex {
			continue
		}
		jobs = append(jobs, articleJobsForPeriod(periodIndex, storeCount)...)
		if periods[periodIndex].includeTrend {
			jobs = append(jobs, analysisJob{kind: "trend", periodIndex: periodIndex, storeIndex: -1})
		}
	}
	return jobs
}

func (a *App) runSalesAnalysisJobs(
	ctx context.Context,
	operationID string,
	selected []selectedStore,
	periods []normalizedSalesAnalysisPeriod,
	tasks []analysisJob,
	progressOffset, totalTasks, concurrency int,
) ([][]storeOutcome, int, error) {
	if len(tasks) == 0 {
		return make([][]storeOutcome, len(periods)), progressOffset, nil
	}
	outcomes := make([][]storeOutcome, len(periods))
	for periodIndex := range periods {
		outcomes[periodIndex] = make([]storeOutcome, len(selected))
	}
	trendOutcomes := make([]storeOutcome, len(periods))
	// Schedule whole authenticated-account lanes. This keeps one login's store
	// queries serial without occupying every worker with callers waiting on the
	// same account, so other accounts can continue independently.
	jobsByLane := make(map[string][]analysisJob)
	laneOrder := make([]string, 0)
	for _, task := range tasks {
		lane := salesAnalysisJobLane(task, selected)
		if _, exists := jobsByLane[lane]; !exists {
			laneOrder = append(laneOrder, lane)
		}
		jobsByLane[lane] = append(jobsByLane[lane], task)
	}
	jobs := make(chan []analysisJob)
	workerCount := min(concurrency, len(laneOrder))
	var waitGroup sync.WaitGroup
	var completed atomic.Int64
	completed.Store(int64(progressOffset))
	for worker := 0; worker < workerCount; worker++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for lane := range jobs {
				for _, task := range lane {
					period := periods[task.periodIndex]
					query := rtasales.SalesQuery{
						StartDate:         period.from,
						EndDate:           period.to,
						Category:          "全部商品",
						SkipTrendLookback: period.key != "current",
						Compact:           true,
					}
					storeID, storeLabel := "", "全部"
					var result *rtasales.SalesResult
					var queryErr error
					if task.kind == "trend" {
						query.SkipArticle = true
						query.AllStores = true
						result, queryErr = a.querySalesAnalysisWithRateLimitRetry(ctx, selected[0].query, query)
						trendOutcomes[task.periodIndex] = storeOutcome{result: result, err: queryErr}
					} else {
						selectedStore := selected[task.storeIndex]
						store := selectedStore.route.store
						storeID = store.BusinessID
						storeLabel = store.Label
						query.BusinessStoreID = store.BusinessID
						query.SkipTrend = true
						result, queryErr = a.querySalesAnalysisWithRateLimitRetry(ctx, selectedStore.query, query)
						outcomes[task.periodIndex][task.storeIndex] = storeOutcome{result: result, err: queryErr}
					}
					current := int(completed.Add(1))
					status := "success"
					if queryErr != nil {
						status = "failed"
					}
					a.events.Emit(a.appContext(), salesAnalysisProgressEventName, SalesAnalysisProgress{
						OperationID: operationID, Current: current, Total: totalTasks,
						StoreID: storeID, StoreLabel: storeLabel,
						PeriodKey: period.key, PeriodLabel: period.label, Status: status,
					})
				}
			}
		}()
	}
	sendCancelled := false
	for _, lane := range laneOrder {
		select {
		case jobs <- jobsByLane[lane]:
		case <-ctx.Done():
			sendCancelled = true
		}
		if sendCancelled {
			break
		}
	}
	close(jobs)
	waitGroup.Wait()
	if err := ctx.Err(); err != nil {
		return nil, int(completed.Load()), err
	}
	for periodIndex, outcome := range trendOutcomes {
		if outcome.result == nil && outcome.err == nil {
			continue
		}
		if len(outcomes[periodIndex]) == 0 {
			outcomes[periodIndex] = make([]storeOutcome, 1)
		}
		// Stash the all-stores trend on a sidecar by overwriting nothing;
		// callers read it via applyStoredTrend below using period extras.
		_ = outcome
	}
	return attachTrendOutcomes(outcomes, trendOutcomes), int(completed.Load()), nil
}

func salesAnalysisJobLane(task analysisJob, selected []selectedStore) string {
	route := selected[0].route
	if task.kind != "trend" {
		route = selected[task.storeIndex].route
	}
	if route.lane != "" {
		return route.lane
	}
	if route.profileID != "" {
		return "profile:" + route.profileID
	}
	return "default"
}

func (a *App) querySalesAnalysisWithRateLimitRetry(
	ctx context.Context,
	client accountClient,
	query rtasales.SalesQuery,
) (*rtasales.SalesResult, error) {
	backoff := a.salesAnalysisBackoff
	if backoff == nil {
		backoff = waitForSalesAnalysisRetry
	}
	for attempt := 0; ; attempt++ {
		result, err := client.Sales(ctx, query)
		if err == nil || !isRateLimitError(err) || attempt >= len(salesAnalysisRateLimitRetryDelays) {
			return result, err
		}
		if waitErr := backoff(ctx, salesAnalysisRateLimitRetryDelays[attempt]); waitErr != nil {
			return nil, waitErr
		}
	}
}

func isRateLimitError(err error) bool {
	var upstream *rtasales.UpstreamError
	return errors.As(err, &upstream) && upstream.StatusCode == 429
}

func waitForSalesAnalysisRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func attachTrendOutcomes(article [][]storeOutcome, trends []storeOutcome) [][]storeOutcome {
	for periodIndex, trend := range trends {
		if trend.result == nil && trend.err == nil {
			continue
		}
		article[periodIndex] = append(article[periodIndex], trend)
	}
	return article
}

func (a *App) supplementSalesAnalysis(
	ctx context.Context,
	finish func(),
	started time.Time,
	operationID string,
	selected []selectedStore,
	periods []normalizedSalesAnalysisPeriod,
	primaryIndex int,
	jobs []analysisJob,
	progressOffset, totalTasks, concurrency int,
) {
	defer finish()
	outcomes, _, err := a.runSalesAnalysisJobs(ctx, operationID, selected, periods, jobs, progressOffset, totalTasks, concurrency)
	if err != nil {
		return
	}
	a.mergeSalesAnalysisSupplement(operationID, selected, periods, primaryIndex, outcomes, time.Since(started).Milliseconds())
}

func assembleSalesAnalysisArticles(
	operationID string,
	selected []selectedStore,
	periods []normalizedSalesAnalysisPeriod,
	outcomes [][]storeOutcome,
) (SalesAnalysisResult, map[string]SalesAnalysisPackedItems) {
	analysis := SalesAnalysisResult{
		OperationID:    operationID,
		SelectedStores: len(selected),
		Periods:        make([]SalesAnalysisPeriodResult, 0, len(periods)),
		Issues:         make([]SalesAnalysisIssue, 0),
	}
	packed := make(map[string]SalesAnalysisPackedItems, len(periods))
	for periodIndex, period := range periods {
		periodResult, periodPacked := assembleArticlePeriod(period, selected, outcomes[periodIndex])
		packed[period.key] = periodPacked
		analysis.Issues = append(analysis.Issues, periodResult.Issues...)
		analysis.Periods = append(analysis.Periods, periodResult)
	}
	if len(analysis.Periods) > 0 {
		primary := analysis.Periods[0]
		analysis.From = primary.From
		analysis.To = primary.To
		analysis.SuccessfulStores = primary.SuccessfulStores
		analysis.Totals = primary.Totals
		analysis.Stores = primary.Stores
	}
	return analysis, packed
}

func assembleArticlePeriod(
	period normalizedSalesAnalysisPeriod,
	selected []selectedStore,
	outcomes []storeOutcome,
) (SalesAnalysisPeriodResult, SalesAnalysisPackedItems) {
	periodResult := SalesAnalysisPeriodResult{
		Key: period.key, Label: period.label,
		From: period.from.Format("2006-01-02"), To: period.to.Format("2006-01-02"),
		Stores: make([]SalesAnalysisStoreSummary, 0, len(selected)),
		Issues: make([]SalesAnalysisIssue, 0),
	}
	builder := newPackedPeriodBuilder()
	articleCount := min(len(outcomes), len(selected))
	for storeIndex := 0; storeIndex < articleCount; storeIndex++ {
		store := selected[storeIndex].route.store
		outcome := outcomes[storeIndex]
		if outcome.err != nil {
			periodResult.Issues = append(periodResult.Issues, SalesAnalysisIssue{
				PeriodKey: period.key, StoreID: store.BusinessID, StoreLabel: store.Label, Message: outcome.err.Error(),
			})
			continue
		}
		totals, added, conversionErr := builder.appendStore(store, articleOnlyResult(outcome.result))
		if conversionErr != nil {
			periodResult.Issues = append(periodResult.Issues, SalesAnalysisIssue{
				PeriodKey: period.key, StoreID: store.BusinessID, StoreLabel: store.Label, Message: conversionErr.Error(),
			})
			continue
		}
		periodResult.SuccessfulStores++
		periodResult.ItemCount += added
		periodResult.Stores = append(periodResult.Stores, SalesAnalysisStoreSummary{
			BusinessID: store.BusinessID, Label: store.Label, Totals: totals,
		})
		addSalesAnalysisTotals(&periodResult.Totals, totals)
	}
	if trend, ok := periodTrendOutcome(outcomes, len(selected)); ok {
		applyAllStoresTrend(&periodResult, trend)
	}
	periodResult.Complete = len(periodResult.Issues) == 0
	packed := builder.finish(period.key)
	attachPeriodSummary(&periodResult, packed)
	return periodResult, packed
}

func periodTrendOutcome(outcomes []storeOutcome, storeCount int) (storeOutcome, bool) {
	if len(outcomes) > storeCount {
		return outcomes[storeCount], true
	}
	return storeOutcome{}, false
}

func applyAllStoresTrend(period *SalesAnalysisPeriodResult, outcome storeOutcome) {
	if outcome.err != nil {
		period.Issues = append(period.Issues, SalesAnalysisIssue{
			PeriodKey: period.Key, StoreID: "", StoreLabel: "全部", Message: outcome.err.Error(),
		})
		return
	}
	if outcome.result == nil {
		return
	}
	period.Totals.TrendNetSalesAmount = outcome.result.TrendGrossSaleAmount
	period.Totals.TransactionCount = outcome.result.TotalTransactionCount
}

func articleOnlyResult(result *rtasales.SalesResult) *rtasales.SalesResult {
	if result == nil {
		return nil
	}
	clone := *result
	clone.TrendGrossSaleAmount = nil
	clone.TotalTransactionCount = nil
	clone.TrendDays = nil
	return &clone
}

func (a *App) mergeSalesAnalysisSupplement(
	operationID string,
	selected []selectedStore,
	periods []normalizedSalesAnalysisPeriod,
	primaryIndex int,
	outcomes [][]storeOutcome,
	durationMS int64,
) {
	a.salesResultMu.Lock()
	if a.salesResult == nil || a.salesResult.OperationID != operationID {
		a.salesResultMu.Unlock()
		return
	}
	current := *a.salesResult
	packed := a.salesPacked
	if packed == nil {
		packed = make(map[string]SalesAnalysisPackedItems)
	}
	byKey := make(map[string]int, len(current.Periods))
	for index, period := range current.Periods {
		byKey[period.Key] = index
	}
	if trend, ok := periodTrendOutcome(outcomes[primaryIndex], len(selected)); ok {
		period := current.Periods[byKey[periods[primaryIndex].key]]
		applyAllStoresTrend(&period, trend)
		period.Complete = len(period.Issues) == 0
		current.Periods[byKey[periods[primaryIndex].key]] = period
		if periods[primaryIndex].key == "current" && trend.err == nil && trend.result != nil {
			current.Weeks = foldSalesAnalysisWeeksForPeriods(periods, outcomes, primaryIndex, len(selected))
		}
	}
	for periodIndex, period := range periods {
		if periodIndex == primaryIndex {
			continue
		}
		periodResult, periodPacked := assembleArticlePeriod(period, selected, outcomes[periodIndex])
		packed[period.key] = periodPacked
		current.Periods = append(current.Periods, periodResult)
		current.Issues = append(current.Issues, periodResult.Issues...)
	}
	if index, ok := byKey["current"]; ok {
		current.SuccessfulStores = current.Periods[index].SuccessfulStores
		current.Totals = current.Periods[index].Totals
		current.Stores = current.Periods[index].Stores
		current.From = current.Periods[index].From
		current.To = current.Periods[index].To
	}
	current.Pending = false
	current.Complete = len(current.Issues) == 0
	current.QueryDurationMS = durationMS
	slim := slimSalesAnalysis(current)
	a.salesResult = &slim
	a.salesPacked = packed
	a.salesResultMu.Unlock()
	a.events.Emit(a.appContext(), salesAnalysisUpdateEventName, slim)
}

func normalizeSalesAnalysisPeriods(request SalesAnalysisRequest) ([]normalizedSalesAnalysisPeriod, error) {
	if len(request.Periods) == 0 {
		from, to, err := parseRequiredRange(strings.TrimSpace(request.From), strings.TrimSpace(request.To))
		if err != nil {
			return nil, err
		}
		return []normalizedSalesAnalysisPeriod{{key: "current", label: "current", from: from, to: to}}, nil
	}
	if len(request.Periods) > 6 {
		return nil, errors.New("sales analysis supports at most 6 periods")
	}
	seen := make(map[string]struct{}, len(request.Periods))
	periods := make([]normalizedSalesAnalysisPeriod, 0, len(request.Periods))
	for index, source := range request.Periods {
		key := strings.TrimSpace(source.Key)
		if key == "" || len(key) > 40 {
			return nil, fmt.Errorf("period %d has an invalid key", index+1)
		}
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("period key %q is duplicated", key)
		}
		seen[key] = struct{}{}
		label := strings.TrimSpace(source.Label)
		if label == "" {
			label = key
		}
		if len(label) > 80 {
			return nil, fmt.Errorf("period %q label is too long", key)
		}
		from, to, err := parseRequiredRange(strings.TrimSpace(source.From), strings.TrimSpace(source.To))
		if err != nil {
			return nil, fmt.Errorf("period %q: %w", key, err)
		}
		periods = append(periods, normalizedSalesAnalysisPeriod{
			key: key, label: label, from: from, to: to, includeTrend: source.IncludeTrend,
		})
	}
	return periods, nil
}

func finalizeSalesAnalysisTrendTotals(totals *SalesAnalysisTotals, stores []SalesAnalysisStoreSummary) {
	if len(stores) == 0 {
		totals.TrendNetSalesAmount = nil
		totals.TransactionCount = nil
		return
	}
	transactionTotal := 0.0
	trendNetSalesTotal := 0.0
	transactionsComplete := true
	trendNetSalesComplete := true
	for _, store := range stores {
		if store.Totals.TrendNetSalesAmount == nil {
			trendNetSalesComplete = false
		} else {
			trendNetSalesTotal += *store.Totals.TrendNetSalesAmount
		}
		if store.Totals.TransactionCount == nil {
			transactionsComplete = false
		} else {
			transactionTotal += *store.Totals.TransactionCount
		}
	}
	if trendNetSalesComplete {
		totals.TrendNetSalesAmount = &trendNetSalesTotal
	} else {
		totals.TrendNetSalesAmount = nil
	}
	if transactionsComplete {
		totals.TransactionCount = &transactionTotal
	} else {
		totals.TransactionCount = nil
	}
}

func (a *App) CancelSalesAnalysis(request OperationRequest) error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	if !a.salesAnalysisRunning || a.salesAnalysisCancel == nil || a.salesAnalysisID != strings.TrimSpace(request.OperationID) {
		return errors.New("sales analysis operation does not exist")
	}
	a.salesAnalysisCancel()
	return nil
}

func (a *App) beginSalesAnalysisOperation(operationID string) (context.Context, func(), error) {
	a.operationMu.Lock()
	if (a.active != nil && a.active.running) || a.profileMutationRunning || a.profileTestRunning || a.salesAnalysisRunning {
		a.operationMu.Unlock()
		return nil, nil, errors.New("another account or workbook operation is already running")
	}
	ctx, cancel := context.WithCancel(a.appContext())
	a.salesAnalysisRunning = true
	a.salesAnalysisID = operationID
	a.salesAnalysisCancel = cancel
	a.operationMu.Unlock()
	return ctx, func() {
		cancel()
		a.operationMu.Lock()
		if a.salesAnalysisID == operationID {
			a.salesAnalysisRunning = false
			a.salesAnalysisID = ""
			a.salesAnalysisCancel = nil
		}
		a.operationMu.Unlock()
	}, nil
}

type analysisStoreRoute struct {
	store     rtasales.Store
	client    accountClient
	lane      string
	profileID string
	profile   string
}

func (a *App) salesAnalysisRoutes(ctx context.Context, profileID string, simulateStoreCount int) ([]analysisStoreRoute, error) {
	profiles, err := a.salesAnalysisProfiles(profileID)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	routes := make([]analysisStoreRoute, 0)
	for _, profile := range profiles {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		session, err := a.salesAnalysisAccountClient(profile.ID)
		if err != nil {
			if strings.TrimSpace(profileID) == "" && errors.Is(err, securestore.ErrNotFound) {
				continue
			}
			return nil, err
		}
		client := session.client
		stores, err := client.Stores(ctx)
		if err != nil {
			return nil, err
		}
		for _, store := range stores {
			storeID := strings.TrimSpace(store.BusinessID)
			if storeID == "" {
				continue
			}
			if _, exists := seen[storeID]; exists {
				continue
			}
			seen[storeID] = struct{}{}
			store.BusinessID = storeID
			store.Label = strings.TrimSpace(store.Label)
			routes = append(routes, analysisStoreRoute{
				store: store, client: client, lane: session.lane, profileID: profile.ID, profile: profile.DisplayName,
			})
		}
	}
	if len(routes) == 0 {
		return nil, errors.New("no authorized stores are available")
	}
	return expandAnalysisStoreRoutes(routes, simulateStoreCount), nil
}

func expandAnalysisStoreRoutes(routes []analysisStoreRoute, simulateStoreCount int) []analysisStoreRoute {
	if len(routes) == 0 {
		return routes
	}
	stores := make([]rtasales.Store, 0, len(routes))
	byID := make(map[string]analysisStoreRoute, len(routes))
	for _, route := range routes {
		stores = append(stores, route.store)
		byID[route.store.BusinessID] = route
	}
	expanded := expandSimulatedStores(stores, simulateStoreCount)
	result := make([]analysisStoreRoute, 0, len(expanded))
	for _, store := range expanded {
		sourceID, _, _ := resolveSimulatedStore(store.BusinessID)
		source, ok := byID[sourceID]
		if !ok {
			continue
		}
		result = append(result, analysisStoreRoute{
			store: store, client: source.client, lane: source.lane, profileID: source.profileID, profile: source.profile,
		})
	}
	return result
}

func (a *App) salesAnalysisProfiles(profileID string) ([]profileRecord, error) {
	profileID = strings.TrimSpace(profileID)
	a.profileMu.Lock()
	records, err := a.profiles.List()
	a.profileMu.Unlock()
	if err != nil {
		return nil, err
	}
	if profileID != "" {
		if !validProfileID(profileID) {
			return nil, errors.New("invalid profile identifier")
		}
		index, ok := findProfile(records, profileID)
		if !ok {
			return nil, errors.New("profile does not exist")
		}
		if !records[index].Enabled {
			return nil, errors.New("profile must be enabled before running sales analysis")
		}
		return []profileRecord{records[index]}, nil
	}
	enabled := make([]profileRecord, 0, len(records))
	for _, record := range records {
		if record.Enabled {
			enabled = append(enabled, record)
		}
	}
	if len(enabled) == 0 {
		return nil, errors.New("at least one enabled profile is required")
	}
	return enabled, nil
}

type accountSession struct {
	client accountClient
	lane   string
}

func (a *App) salesAnalysisAccountClient(profileID string) (accountSession, error) {
	credential, err := a.credentials.Get(profileID)
	if errors.Is(err, securestore.ErrNotFound) {
		return accountSession{}, fmt.Errorf("profile has no saved credentials: %w", err)
	}
	if err != nil {
		return accountSession{}, err
	}
	cookies, err := a.cookies.CookieStore(profileID)
	if err != nil {
		return accountSession{}, err
	}
	client, err := a.clients.New(credential, cookies)
	if err != nil {
		return accountSession{}, err
	}
	return accountSession{client: client, lane: accountLane(credential.Account)}, nil
}

func accountLane(account string) string {
	digest := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(account))))
	return fmt.Sprintf("account:%x", digest[:])
}

func normalizeSalesAnalysisStoreIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func salesAnalysisRows(store rtasales.Store, result *rtasales.SalesResult) ([]SalesAnalysisItem, SalesAnalysisTotals, error) {
	if result == nil {
		return nil, SalesAnalysisTotals{}, errors.New("RTA returned no sales result")
	}
	items := make([]SalesAnalysisItem, 0, len(result.Items))
	totals := SalesAnalysisTotals{}
	for _, source := range result.Items {
		item := SalesAnalysisItem{
			StoreID: store.BusinessID, StoreLabel: store.Label,
			Category1: source.PurchaseCategory1Name, Category1Code: source.PurchaseCategory1Code,
			Category2: source.PurchaseCategory2Name, Category2Code: source.PurchaseCategory2Code,
			Category3: source.PurchaseCategory3Name, Category3Code: source.PurchaseCategory3Code,
			Category4: source.PurchaseCategory4Name, Category4Code: source.PurchaseCategory4Code,
			Category5: source.PurchaseCategory5Name, Category5Code: source.PurchaseCategory5Code,
			ArticleCode: source.Matnr, ArticleName: source.ArticleName, BrandName: source.BrandName,
			TransactionCount:       countValue(source.TPTransactionCount),
			SaleQuantity:           countValue(source.TPSaleQuantity),
			SaleAmount:             source.TPSaleAmount,
			ReturnTransactionCount: countValue(source.TPReturnTransactionCount),
			ReturnQuantity:         countValue(source.TPReturnSaleQuantity),
			ReturnAmount:           source.TPReturnSaleAmount,
			NetQuantity:            countValue(source.TPGrossSaleQuantity),
			NetSalesAmount:         source.TPGrossSaleAmount,
		}
		if !finiteSalesAnalysisItem(item) {
			return nil, SalesAnalysisTotals{}, fmt.Errorf("article %q contains a non-finite sales value", source.Matnr)
		}
		items = append(items, item)
		totals.SaleQuantity += item.SaleQuantity
		totals.SaleAmount += item.SaleAmount
		totals.ReturnQuantity += item.ReturnQuantity
		totals.ReturnAmount += item.ReturnAmount
		totals.NetQuantity += item.NetQuantity
		totals.NetSalesAmount += item.NetSalesAmount
	}
	if result.TotalTransactionCount != nil {
		if !finite(*result.TotalTransactionCount) {
			return nil, SalesAnalysisTotals{}, errors.New("RTA returned a non-finite transaction count")
		}
		transactionCount := countValue(*result.TotalTransactionCount)
		totals.TransactionCount = &transactionCount
	}
	if result.TrendGrossSaleAmount != nil {
		if !finite(*result.TrendGrossSaleAmount) {
			return nil, SalesAnalysisTotals{}, errors.New("RTA returned a non-finite Trend View net sales amount")
		}
		trendNetSalesAmount := *result.TrendGrossSaleAmount
		totals.TrendNetSalesAmount = &trendNetSalesAmount
	}
	return items, totals, nil
}

func countValue(value float64) float64 {
	return math.Round(value)
}

func addSalesAnalysisTotals(destination *SalesAnalysisTotals, source SalesAnalysisTotals) {
	destination.SaleQuantity += source.SaleQuantity
	destination.SaleAmount += source.SaleAmount
	destination.ReturnQuantity += source.ReturnQuantity
	destination.ReturnAmount += source.ReturnAmount
	destination.NetQuantity += source.NetQuantity
	destination.NetSalesAmount += source.NetSalesAmount
}

func finiteSalesAnalysisItem(item SalesAnalysisItem) bool {
	return finite(item.TransactionCount) && finite(item.SaleQuantity) && finite(item.SaleAmount) &&
		finite(item.ReturnTransactionCount) && finite(item.ReturnQuantity) && finite(item.ReturnAmount) &&
		finite(item.NetQuantity) && finite(item.NetSalesAmount)
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
