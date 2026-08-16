package desktop

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	rtasales "github.com/Miku0139oao/rta-sales-client-go"
	"github.com/Miku0139oao/rta-sales-client-go/securestore"
	"github.com/Miku0139oao/rta-sales-client-go/xlsxfill"
)

const salesAnalysisProgressEventName = "rta:sales-analysis-progress"

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

// RunSalesAnalysis queries selected stores in parallel. Each store uses the
// account that owns it so multiple profiles can request RTA at the same time.
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
	defer finish()

	routes, err := a.salesAnalysisRoutes(ctx, request.ProfileID, request.SimulateStoreCount)
	if err != nil {
		return SalesAnalysisResult{}, err
	}
	byID := make(map[string]analysisStoreRoute, len(routes))
	for _, route := range routes {
		byID[strings.TrimSpace(route.store.BusinessID)] = route
	}
	type selectedStore struct {
		route analysisStoreRoute
		query accountClient
	}
	selected := make([]selectedStore, 0, len(storeIDs))
	for _, storeID := range storeIDs {
		route, ok := byID[storeID]
		if !ok {
			return SalesAnalysisResult{}, fmt.Errorf("store %q is not authorized for the selected profiles", storeID)
		}
		selected = append(selected, selectedStore{
			route: route,
			query: maybeSimulateClient(route.client, request.SimulateStoreCount),
		})
	}
	type storeOutcome struct {
		result *rtasales.SalesResult
		err    error
	}
	type queryTask struct {
		periodIndex int
		storeIndex  int
	}
	outcomes := make([][]storeOutcome, len(periods))
	tasks := make([]queryTask, 0, len(periods)*len(selected))
	for periodIndex := range periods {
		outcomes[periodIndex] = make([]storeOutcome, len(selected))
		for storeIndex := range selected {
			tasks = append(tasks, queryTask{periodIndex: periodIndex, storeIndex: storeIndex})
		}
	}
	jobs := make(chan queryTask)
	workerCount := min(concurrency, len(tasks))
	var waitGroup sync.WaitGroup
	var completed atomic.Int64
	a.events.Emit(a.appContext(), salesAnalysisProgressEventName, SalesAnalysisProgress{
		OperationID: operationID,
		Current:     0,
		Total:       len(tasks),
		Status:      "running",
	})
	for worker := 0; worker < workerCount; worker++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for task := range jobs {
				selectedStore := selected[task.storeIndex]
				store := selectedStore.route.store
				period := periods[task.periodIndex]
				result, queryErr := selectedStore.query.Sales(ctx, rtasales.SalesQuery{
					BusinessStoreID: store.BusinessID,
					StartDate:       period.from,
					EndDate:         period.to,
					Category:        "全部商品",
					SkipTrend:           !period.includeTrend,
					SkipTrendLookback:   period.key != "current",
					Compact:             true,
				})
				outcomes[task.periodIndex][task.storeIndex] = storeOutcome{result: result, err: queryErr}
				current := int(completed.Add(1))
				status := "success"
				if queryErr != nil {
					status = "failed"
				}
				a.events.Emit(a.appContext(), salesAnalysisProgressEventName, SalesAnalysisProgress{
					OperationID: operationID,
					Current:     current,
					Total:       len(tasks),
					StoreID:     store.BusinessID,
					StoreLabel:  store.Label,
					PeriodKey:   period.key,
					PeriodLabel: period.label,
					Status:      status,
				})
			}
		}()
	}
	sendCancelled := false
	for _, task := range tasks {
		select {
		case jobs <- task:
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
		return SalesAnalysisResult{}, err
	}

	analysis := SalesAnalysisResult{
		OperationID:    operationID,
		SelectedStores: len(selected),
		Periods:        make([]SalesAnalysisPeriodResult, 0, len(periods)),
		Issues:         make([]SalesAnalysisIssue, 0),
	}
	packed := make(map[string]SalesAnalysisPackedItems, len(periods))
	var currentTrend []storeTrendSeries
	for periodIndex, period := range periods {
		periodResult := SalesAnalysisPeriodResult{
			Key: period.key, Label: period.label,
			From: period.from.Format("2006-01-02"), To: period.to.Format("2006-01-02"),
			Stores: make([]SalesAnalysisStoreSummary, 0, len(selected)),
			Issues: make([]SalesAnalysisIssue, 0),
		}
		builder := newPackedPeriodBuilder()
		for storeIndex, outcome := range outcomes[periodIndex] {
			store := selected[storeIndex].route.store
			if outcome.err != nil {
				periodResult.Issues = append(periodResult.Issues, SalesAnalysisIssue{
					PeriodKey: period.key, StoreID: store.BusinessID, StoreLabel: store.Label, Message: outcome.err.Error(),
				})
				continue
			}
			totals, added, conversionErr := builder.appendStore(store, outcome.result)
			outcomes[periodIndex][storeIndex].result = nil
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
			if period.key == "current" && outcome.result != nil {
				currentTrend = append(currentTrend, storeTrendSeries{store: store, days: outcome.result.TrendDays})
			}
		}
		periodResult.Complete = len(periodResult.Issues) == 0
		finalizeSalesAnalysisTrendTotals(&periodResult.Totals, periodResult.Stores)
		packed[period.key] = builder.finish(period.key)
		analysis.Issues = append(analysis.Issues, periodResult.Issues...)
		analysis.Periods = append(analysis.Periods, periodResult)
	}
	primary := analysis.Periods[0]
	analysis.From = primary.From
	analysis.To = primary.To
	analysis.SuccessfulStores = primary.SuccessfulStores
	analysis.Totals = primary.Totals
	analysis.Stores = primary.Stores
	for _, period := range periods {
		if period.key == "current" {
			analysis.Weeks = foldSalesAnalysisWeeks(period.from, period.to, currentTrend)
			break
		}
	}
	analysis.Complete = len(analysis.Issues) == 0
	analysis.QueryDurationMS = time.Since(started).Milliseconds()
	return a.rememberSalesAnalysis(analysis, packed), nil
}

type normalizedSalesAnalysisPeriod struct {
	key          string
	label        string
	from         time.Time
	to           time.Time
	includeTrend bool
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
		client, err := a.salesAnalysisAccountClient(profile.ID)
		if err != nil {
			if strings.TrimSpace(profileID) == "" && errors.Is(err, securestore.ErrNotFound) {
				continue
			}
			return nil, err
		}
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
				store: store, client: client, profileID: profile.ID, profile: profile.DisplayName,
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
			store: store, client: source.client, profileID: source.profileID, profile: source.profile,
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

func (a *App) salesAnalysisAccountClient(profileID string) (accountClient, error) {
	credential, err := a.credentials.Get(profileID)
	if errors.Is(err, securestore.ErrNotFound) {
		return nil, fmt.Errorf("profile has no saved credentials: %w", err)
	}
	if err != nil {
		return nil, err
	}
	cookies, err := a.cookies.CookieStore(profileID)
	if err != nil {
		return nil, err
	}
	return a.clients.New(credential, cookies)
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
