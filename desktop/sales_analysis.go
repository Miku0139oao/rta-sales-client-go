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
	// maxAccountQuerySessions is how many independent RTA logins one profile
	// may open. Each login keeps its own cookie; requests on one cookie stay
	// serial. RTA does not evict older sessions, so one store can have its
	// own login up to the workbook concurrency ceiling.
	maxAccountQuerySessions = xlsxfill.MaximumConcurrency
)

var salesAnalysisRateLimitRetryDelays = [...]time.Duration{time.Second, 3 * time.Second}

// ListSalesAnalysisStores loads authorized stores from one profile, or from
// every enabled profile when ProfileID is empty. Overlapping stores keep the
// earlier profile in account priority order.
func (a *App) ListSalesAnalysisStores(request ProfileIDRequest) ([]SalesAnalysisStore, error) {
	releaseAdmission, admissionErr := a.admitWork()
	if admissionErr != nil {
		return nil, admissionErr
	}
	defer releaseAdmission()
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
	releaseAdmission, admissionErr := a.admitWork()
	if admissionErr != nil {
		return SalesAnalysisResult{}, admissionErr
	}
	defer releaseAdmission()
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
	if err := a.spreadAccountQuerySessions(selected, concurrency, request.SimulateStoreCount); err != nil {
		return SalesAnalysisResult{}, err
	}
	primaryIndex := primarySalesAnalysisPeriodIndex(periods)
	primaryJobs := articleJobsForPeriod(primaryIndex, len(selected))
	followJobs := followOnSalesAnalysisJobs(periods, primaryIndex, len(selected))
	totalTasks := len(primaryJobs) + len(followJobs)

	a.events.Emit(a.appContext(), salesAnalysisProgressEventName, SalesAnalysisProgress{
		OperationID: operationID, Current: 0, Total: totalTasks, Status: "running",
	})
	run := a.startSalesAnalysisJobs(ctx, operationID, selected, periods, primaryJobs, followJobs, 0, totalTasks, concurrency, primaryIndex)
	select {
	case <-run.primaryDone:
	case <-ctx.Done():
	}
	if err := ctx.Err(); err != nil {
		_ = run.wait()
		return SalesAnalysisResult{}, err
	}

	analysis, packed := assembleSalesAnalysisArticles(
		operationID, selected,
		[]normalizedSalesAnalysisPeriod{periods[primaryIndex]},
		[][]storeOutcome{run.copyPeriodArticles(primaryIndex, len(selected))},
	)
	analysis.Pending = len(followJobs) > 0
	analysis.Complete = !analysis.Pending && len(analysis.Issues) == 0
	analysis.QueryDurationMS = time.Since(started).Milliseconds()
	remembered := a.rememberSalesAnalysis(analysis, packed)
	if len(followJobs) == 0 {
		if err := run.wait(); err != nil {
			return SalesAnalysisResult{}, err
		}
		released = true
		finish()
		return remembered, nil
	}

	released = true
	go a.finishSalesAnalysisSupplement(finish, started, operationID, selected, periods, primaryIndex, run)
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

type analysisJobRun struct {
	// Article slice headers never change after setup. primaryDone publishes the
	// current-period rows; done separately publishes finalOutcomes with trends.
	finalOutcomes [][]storeOutcome
	outcomes      [][]storeOutcome
	completed     atomic.Int64
	primaryDone   chan struct{}
	done          chan struct{}
	runErr        error
	errMu         sync.Mutex
}

func (r *analysisJobRun) wait() error {
	<-r.done
	r.errMu.Lock()
	defer r.errMu.Unlock()
	return r.runErr
}

func (r *analysisJobRun) copyPeriodArticles(periodIndex, storeCount int) []storeOutcome {
	if r == nil || periodIndex < 0 || periodIndex >= len(r.outcomes) {
		return make([]storeOutcome, storeCount)
	}
	out := make([]storeOutcome, storeCount)
	copy(out, r.outcomes[periodIndex])
	return out
}

func isPrimaryArticleJob(task analysisJob, primaryIndex int) bool {
	return task.kind == "article" && task.periodIndex == primaryIndex
}

func (a *App) startSalesAnalysisJobs(
	ctx context.Context,
	operationID string,
	selected []selectedStore,
	periods []normalizedSalesAnalysisPeriod,
	primaryJobs, followJobs []analysisJob,
	progressOffset, totalTasks, concurrency, primaryIndex int,
) *analysisJobRun {
	run := &analysisJobRun{
		outcomes:    make([][]storeOutcome, len(periods)),
		primaryDone: make(chan struct{}),
		done:        make(chan struct{}),
	}
	run.completed.Store(int64(progressOffset))
	for periodIndex := range periods {
		run.outcomes[periodIndex] = make([]storeOutcome, len(selected))
	}
	tasks := append(append([]analysisJob{}, primaryJobs...), followJobs...)
	if len(tasks) == 0 {
		run.finalOutcomes = run.outcomes
		close(run.primaryDone)
		close(run.done)
		return run
	}
	trendOutcomes := make([]storeOutcome, len(periods))
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
	var primaryOnce sync.Once
	primaryRemaining := atomic.Int64{}
	primaryRemaining.Store(int64(len(primaryJobs)))
	if len(primaryJobs) == 0 {
		primaryOnce.Do(func() { close(run.primaryDone) })
	}
	markPrimary := func(task analysisJob) {
		if !isPrimaryArticleJob(task, primaryIndex) {
			return
		}
		if primaryRemaining.Add(-1) == 0 {
			primaryOnce.Do(func() { close(run.primaryDone) })
		}
	}
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
						result, queryErr = a.querySelectedStoresTrend(ctx, selected, period)
						trendOutcomes[task.periodIndex] = storeOutcome{result: result, err: queryErr}
					} else {
						selectedStore := selected[task.storeIndex]
						store := selectedStore.route.store
						storeID = store.BusinessID
						storeLabel = store.Label
						query.BusinessStoreID = store.BusinessID
						query.SkipTrend = true
						result, queryErr = a.querySalesAnalysisWithRateLimitRetry(ctx, selectedStore.query, query)
						run.outcomes[task.periodIndex][task.storeIndex] = storeOutcome{result: result, err: queryErr}
					}
					current := int(run.completed.Add(1))
					status := "success"
					if queryErr != nil {
						status = "failed"
					}
					a.events.Emit(a.appContext(), salesAnalysisProgressEventName, SalesAnalysisProgress{
						OperationID: operationID, Current: current, Total: totalTasks,
						StoreID: storeID, StoreLabel: storeLabel,
						PeriodKey: period.key, PeriodLabel: period.label, Status: status,
					})
					markPrimary(task)
				}
			}
		}()
	}
	for _, lane := range laneOrder {
		select {
		case jobs <- jobsByLane[lane]:
		case <-ctx.Done():
			close(jobs)
			go func() {
				waitGroup.Wait()
				primaryOnce.Do(func() { close(run.primaryDone) })
				run.errMu.Lock()
				run.runErr = ctx.Err()
				run.errMu.Unlock()
				close(run.done)
			}()
			return run
		}
	}
	close(jobs)
	go func() {
		waitGroup.Wait()
		primaryOnce.Do(func() { close(run.primaryDone) })
		run.finalOutcomes = attachTrendOutcomes(run.outcomes, trendOutcomes)
		if err := ctx.Err(); err != nil {
			run.errMu.Lock()
			run.runErr = err
			run.errMu.Unlock()
		}
		close(run.done)
	}()
	return run
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

func (a *App) querySelectedStoresTrend(ctx context.Context, selected []selectedStore, period normalizedSalesAnalysisPeriod) (*rtasales.SalesResult, error) {
	if len(selected) == 0 {
		return nil, errors.New("at least one store is required")
	}
	base := rtasales.SalesQuery{
		StartDate:         period.from,
		EndDate:           period.to,
		Category:          "全部商品",
		SkipArticle:       true,
		SkipTrendLookback: period.key != "current",
		Compact:           true,
	}
	var results []*rtasales.SalesResult
	for _, group := range groupSelectedStoresByProfile(selected) {
		covers, err := selectionCoversAccount(ctx, group)
		if err != nil {
			return nil, err
		}
		if covers {
			query := base
			query.AllStores = true
			result, queryErr := a.querySalesAnalysisWithRateLimitRetry(ctx, group[0].query, query)
			if queryErr != nil {
				return nil, queryErr
			}
			results = append(results, result)
			continue
		}
		for _, store := range group {
			query := base
			query.BusinessStoreID = store.route.store.BusinessID
			result, queryErr := a.querySalesAnalysisWithRateLimitRetry(ctx, store.query, query)
			if queryErr != nil {
				return nil, queryErr
			}
			results = append(results, result)
		}
	}
	return mergeSelectedTrendResults(results), nil
}

func groupSelectedStoresByProfile(selected []selectedStore) [][]selectedStore {
	indexes := make(map[string]int, len(selected))
	groups := make([][]selectedStore, 0)
	for _, store := range selected {
		key := store.route.profileID
		if key == "" {
			key = "default"
		}
		if index, ok := indexes[key]; ok {
			groups[index] = append(groups[index], store)
			continue
		}
		indexes[key] = len(groups)
		groups = append(groups, []selectedStore{store})
	}
	return groups
}

func selectionCoversAccount(ctx context.Context, group []selectedStore) (bool, error) {
	if len(group) == 0 {
		return false, nil
	}
	authorized, err := group[0].query.Stores(ctx)
	if err != nil {
		return false, err
	}
	selected := make(map[string]struct{}, len(group))
	for _, store := range group {
		id := strings.TrimSpace(store.route.store.BusinessID)
		if id != "" {
			selected[id] = struct{}{}
		}
	}
	if len(authorized) == 0 || len(selected) == 0 {
		return false, nil
	}
	for _, store := range authorized {
		id := strings.TrimSpace(store.BusinessID)
		if id == "" {
			continue
		}
		if _, ok := selected[id]; !ok {
			return false, nil
		}
	}
	return true, nil
}

func mergeSelectedTrendResults(results []*rtasales.SalesResult) *rtasales.SalesResult {
	merged := &rtasales.SalesResult{Store: rtasales.Store{Label: "全部"}}
	if len(results) == 0 {
		return merged
	}
	var sales, tickets float64
	hasSales, hasTickets := false, false
	for _, result := range results {
		if result == nil {
			continue
		}
		if result.TrendGrossSaleAmount != nil {
			sales += *result.TrendGrossSaleAmount
			hasSales = true
		}
		if result.TotalTransactionCount != nil {
			tickets += *result.TotalTransactionCount
			hasTickets = true
		}
		merged.TrendDays = append(merged.TrendDays, result.TrendDays...)
	}
	if hasSales {
		merged.TrendGrossSaleAmount = &sales
	}
	if hasTickets {
		merged.TotalTransactionCount = &tickets
	}
	return merged
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
	combined := append([][]storeOutcome(nil), article...)
	for periodIndex, trend := range trends {
		if trend.result == nil && trend.err == nil {
			continue
		}
		combined[periodIndex] = append(append([]storeOutcome(nil), article[periodIndex]...), trend)
	}
	return combined
}

func (a *App) finishSalesAnalysisSupplement(
	finish func(),
	started time.Time,
	operationID string,
	selected []selectedStore,
	periods []normalizedSalesAnalysisPeriod,
	primaryIndex int,
	run *analysisJobRun,
) {
	defer finish()
	if err := run.wait(); err != nil {
		a.failSalesAnalysisSupplement(operationID, err, time.Since(started).Milliseconds())
		return
	}
	a.mergeSalesAnalysisSupplement(operationID, selected, periods, primaryIndex, run.finalOutcomes, time.Since(started).Milliseconds())
}

// Clone only fields changed during supplementation. Published snapshots can be
// serialized outside salesResultMu, so locking the cache alone is insufficient.
func salesAnalysisForUpdate(current SalesAnalysisResult) SalesAnalysisResult {
	current.Periods = append([]SalesAnalysisPeriodResult(nil), current.Periods...)
	current.Issues = append([]SalesAnalysisIssue(nil), current.Issues...)
	for index := range current.Periods {
		current.Periods[index].Issues = append([]SalesAnalysisIssue(nil), current.Periods[index].Issues...)
	}
	return current
}

func (a *App) failSalesAnalysisSupplement(operationID string, runErr error, durationMS int64) {
	a.salesResultMu.Lock()
	if a.salesResult == nil || a.salesResult.OperationID != operationID {
		a.salesResultMu.Unlock()
		return
	}
	current := salesAnalysisForUpdate(*a.salesResult)
	current.Pending = false
	current.Complete = false
	current.QueryDurationMS = durationMS
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		current.Issues = append(current.Issues, SalesAnalysisIssue{Message: runErr.Error()})
	}
	slim := slimSalesAnalysis(current)
	a.salesResult = &slim
	a.salesResultMu.Unlock()
	a.events.Emit(a.appContext(), salesAnalysisUpdateEventName, slim)
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
	current := salesAnalysisForUpdate(*a.salesResult)
	packed := a.salesPacked
	if packed == nil {
		packed = make(map[string]SalesAnalysisPackedItems)
	}
	byKey := make(map[string]int, len(current.Periods))
	for index, period := range current.Periods {
		byKey[period.Key] = index
	}
	if trend, ok := periodTrendOutcome(outcomes[primaryIndex], len(selected)); ok {
		if index, exists := byKey[periods[primaryIndex].key]; exists {
			period := current.Periods[index]
			applyAllStoresTrend(&period, trend)
			period.Complete = len(period.Issues) == 0
			current.Periods[index] = period
			if periods[primaryIndex].key == "current" && trend.err == nil && trend.result != nil {
				current.Weeks = foldSalesAnalysisWeeksForPeriods(periods, outcomes, primaryIndex, len(selected))
			}
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
	if a.updateReserved {
		a.operationMu.Unlock()
		return nil, nil, errUpdateReserved
	}
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

func accountQuerySessionCount(storeCount, concurrency int) int {
	if storeCount < 1 {
		return 1
	}
	if concurrency < 1 {
		concurrency = 1
	}
	n := concurrency
	if n > maxAccountQuerySessions {
		n = maxAccountQuerySessions
	}
	if n > storeCount {
		n = storeCount
	}
	if n < 1 {
		return 1
	}
	return n
}

func (a *App) extraAccountSessions(primary accountSession, profileID string, count int) ([]accountSession, error) {
	if count < 1 {
		count = 1
	}
	sessions := []accountSession{primary}
	if count == 1 {
		return sessions, nil
	}
	credential, err := a.credentials.Get(profileID)
	if err != nil {
		return nil, err
	}
	for index := 1; index < count; index++ {
		client, err := a.clients.New(credential, new(securestore.MemoryCookieStore))
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, accountSession{
			client: client,
			lane:   fmt.Sprintf("%s:%d", primary.lane, index),
		})
	}
	return sessions, nil
}

func (a *App) spreadAccountQuerySessions(selected []selectedStore, concurrency, simulateStoreCount int) error {
	indexesByProfile := make(map[string][]int)
	order := make([]string, 0)
	for index, store := range selected {
		profileID := store.route.profileID
		if _, exists := indexesByProfile[profileID]; !exists {
			order = append(order, profileID)
		}
		indexesByProfile[profileID] = append(indexesByProfile[profileID], index)
	}
	for _, profileID := range order {
		indexes := indexesByProfile[profileID]
		primary := selected[indexes[0]].route
		sessions, err := a.extraAccountSessions(accountSession{client: primary.client, lane: primary.lane}, profileID, accountQuerySessionCount(len(indexes), concurrency))
		if err != nil {
			return err
		}
		for offset, index := range indexes {
			session := sessions[offset%len(sessions)]
			selected[index].route.client = session.client
			selected[index].route.lane = session.lane
			selected[index].query = maybeSimulateClient(session.client, simulateStoreCount)
		}
	}
	return nil
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
