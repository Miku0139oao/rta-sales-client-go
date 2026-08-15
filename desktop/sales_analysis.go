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

// ListSalesAnalysisStores loads the public stores authorized for one enabled
// desktop profile. The profile's credentials and RTA query-only store keys
// remain in the backend process.
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

	client, err := a.salesAnalysisClient(request.ProfileID)
	if err != nil {
		return nil, err
	}
	stores, err := client.Stores(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]SalesAnalysisStore, 0, len(stores))
	for _, store := range stores {
		businessID := strings.TrimSpace(store.BusinessID)
		if businessID == "" {
			continue
		}
		result = append(result, SalesAnalysisStore{BusinessID: businessID, Label: strings.TrimSpace(store.Label)})
	}
	sort.Slice(result, func(left, right int) bool { return result[left].BusinessID < result[right].BusinessID })
	return result, nil
}

// RunSalesAnalysis queries multiple authorized stores through one account in
// parallel. Article View product rows retain all five category levels so the
// frontend can filter and regroup instantly without another RTA request.
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

	client, err := a.salesAnalysisClient(request.ProfileID)
	if err != nil {
		return SalesAnalysisResult{}, err
	}
	authorized, err := client.Stores(ctx)
	if err != nil {
		return SalesAnalysisResult{}, err
	}
	byID := make(map[string]rtasales.Store, len(authorized))
	for _, store := range authorized {
		byID[strings.TrimSpace(store.BusinessID)] = store
	}
	selected := make([]rtasales.Store, 0, len(storeIDs))
	for _, storeID := range storeIDs {
		store, ok := byID[storeID]
		if !ok {
			return SalesAnalysisResult{}, fmt.Errorf("store %q is not authorized for the selected profile", storeID)
		}
		selected = append(selected, store)
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
				store := selected[task.storeIndex]
				period := periods[task.periodIndex]
				result, queryErr := client.Sales(ctx, rtasales.SalesQuery{
					BusinessStoreID: store.BusinessID,
					StartDate:       period.from,
					EndDate:         period.to,
					Category:        "全部商品",
					SkipTrend:       !period.includeTrend,
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
	for periodIndex, period := range periods {
		periodResult := SalesAnalysisPeriodResult{
			Key: period.key, Label: period.label,
			From: period.from.Format("2006-01-02"), To: period.to.Format("2006-01-02"),
			Stores: make([]SalesAnalysisStoreSummary, 0, len(selected)),
			Items:  make([]SalesAnalysisItem, 0),
			Issues: make([]SalesAnalysisIssue, 0),
		}
		for storeIndex, outcome := range outcomes[periodIndex] {
			store := selected[storeIndex]
			if outcome.err != nil {
				periodResult.Issues = append(periodResult.Issues, SalesAnalysisIssue{
					PeriodKey: period.key, StoreID: store.BusinessID, StoreLabel: store.Label, Message: outcome.err.Error(),
				})
				continue
			}
			items, totals, conversionErr := salesAnalysisRows(store, outcome.result)
			if conversionErr != nil {
				periodResult.Issues = append(periodResult.Issues, SalesAnalysisIssue{
					PeriodKey: period.key, StoreID: store.BusinessID, StoreLabel: store.Label, Message: conversionErr.Error(),
				})
				continue
			}
			periodResult.SuccessfulStores++
			periodResult.Items = append(periodResult.Items, items...)
			periodResult.Stores = append(periodResult.Stores, SalesAnalysisStoreSummary{
				BusinessID: store.BusinessID, Label: store.Label, Totals: totals,
			})
			addSalesAnalysisTotals(&periodResult.Totals, totals)
		}
		periodResult.Complete = len(periodResult.Issues) == 0
		finalizeSalesAnalysisTrendTotals(&periodResult.Totals, periodResult.Stores)
		sortSalesAnalysisItems(periodResult.Items)
		analysis.Issues = append(analysis.Issues, periodResult.Issues...)
		analysis.Periods = append(analysis.Periods, periodResult)
	}
	primary := analysis.Periods[0]
	analysis.From = primary.From
	analysis.To = primary.To
	analysis.SuccessfulStores = primary.SuccessfulStores
	analysis.Totals = primary.Totals
	analysis.Stores = primary.Stores
	analysis.Items = primary.Items
	analysis.Complete = len(analysis.Issues) == 0
	analysis.QueryDurationMS = time.Since(started).Milliseconds()
	return analysis, nil
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

func sortSalesAnalysisItems(items []SalesAnalysisItem) {
	sort.SliceStable(items, func(left, right int) bool {
		if items[left].NetSalesAmount == items[right].NetSalesAmount {
			if items[left].StoreID == items[right].StoreID {
				return items[left].ArticleCode < items[right].ArticleCode
			}
			return items[left].StoreID < items[right].StoreID
		}
		return items[left].NetSalesAmount > items[right].NetSalesAmount
	})
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

func (a *App) salesAnalysisClient(profileID string) (accountClient, error) {
	profileID = strings.TrimSpace(profileID)
	if !validProfileID(profileID) {
		return nil, errors.New("invalid profile identifier")
	}
	a.profileMu.Lock()
	records, err := a.profiles.List()
	if err != nil {
		a.profileMu.Unlock()
		return nil, err
	}
	index, ok := findProfile(records, profileID)
	if !ok {
		a.profileMu.Unlock()
		return nil, errors.New("profile does not exist")
	}
	profile := records[index]
	a.profileMu.Unlock()
	if !profile.Enabled {
		return nil, errors.New("profile must be enabled before running sales analysis")
	}
	credential, err := a.credentials.Get(profileID)
	if errors.Is(err, securestore.ErrNotFound) {
		return nil, errors.New("profile has no saved credentials")
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
			TransactionCount: source.TPTransactionCount,
			SaleQuantity:     source.TPSaleQuantity, SaleAmount: source.TPSaleAmount,
			ReturnTransactionCount: source.TPReturnTransactionCount,
			ReturnQuantity:         source.TPReturnSaleQuantity, ReturnAmount: source.TPReturnSaleAmount,
			NetQuantity: source.TPGrossSaleQuantity, NetSalesAmount: source.TPGrossSaleAmount,
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
		transactionCount := *result.TotalTransactionCount
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
