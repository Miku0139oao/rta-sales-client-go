package desktop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	rtasales "github.com/Miku0139oao/rta-sales-client-go/rtasales"
)

type salesAnalysisFakeClient struct {
	stores  []rtasales.Store
	results map[string]*rtasales.SalesResult
	failOn  func(call int, query rtasales.SalesQuery) error
	hold    func(query rtasales.SalesQuery) <-chan struct{}

	mu        sync.Mutex
	queries   []rtasales.SalesQuery
	active    int
	maxActive int
	started   chan struct{}
	release   chan struct{}
	calls     int
}

func (c *salesAnalysisFakeClient) Stores(context.Context) ([]rtasales.Store, error) {
	return append([]rtasales.Store(nil), c.stores...), nil
}

func (c *salesAnalysisFakeClient) Sales(ctx context.Context, query rtasales.SalesQuery) (*rtasales.SalesResult, error) {
	c.mu.Lock()
	c.queries = append(c.queries, query)
	c.active++
	if c.active > c.maxActive {
		c.maxActive = c.active
	}
	c.mu.Unlock()
	if c.started != nil {
		c.started <- struct{}{}
	}
	var wait <-chan struct{}
	if c.hold != nil {
		wait = c.hold(query)
	}
	if wait == nil && c.release != nil {
		wait = c.release
	}
	if wait != nil {
		select {
		case <-wait:
		case <-ctx.Done():
			c.mu.Lock()
			c.active--
			c.mu.Unlock()
			return nil, ctx.Err()
		}
	}
	c.mu.Lock()
	c.active--
	c.calls++
	call := c.calls
	failOn := c.failOn
	c.mu.Unlock()
	if failOn != nil {
		if err := failOn(call, query); err != nil {
			return nil, err
		}
	}
	if query.AllStores {
		return mergeFakeAllStoresTrend(c.results), nil
	}
	result := c.results[query.BusinessStoreID]
	if result == nil {
		return nil, errors.New("missing fake sales result")
	}
	return result, nil
}

func mergeFakeAllStoresTrend(results map[string]*rtasales.SalesResult) *rtasales.SalesResult {
	merged := &rtasales.SalesResult{}
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

func waitSalesAnalysisSettled(t *testing.T, app *App, operationID string) SalesAnalysisResult {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		app.salesResultMu.Lock()
		result := app.salesResult
		app.salesResultMu.Unlock()
		app.operationMu.Lock()
		running := app.salesAnalysisRunning
		app.operationMu.Unlock()
		if result != nil && result.OperationID == operationID && !result.Pending && !running {
			return *result
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("sales analysis did not finish background work")
	return SalesAnalysisResult{}
}

func TestSalesAnalysisSerializesOneProfileStoresAndPreservesCategories(t *testing.T) {
	transactions107, transactions108 := 10.0, 12.0
	client := &salesAnalysisFakeClient{
		stores: []rtasales.Store{
			{BusinessID: "108", Label: "108 - Second"},
			{BusinessID: "107", Label: "107 - First"},
		},
		results: map[string]*rtasales.SalesResult{
			"107": {
				TotalTransactionCount: &transactions107,
				Items: []rtasales.SaleItem{{
					PurchaseCategory1Name: "A-HEALTH & BEAUTY", PurchaseCategory1Code: "A",
					PurchaseCategory2Name: "BEAUTY CARE", PurchaseCategory2Code: "A02",
					PurchaseCategory3Name: "SKIN CARE", PurchaseCategory3Code: "A0201",
					PurchaseCategory4Name: "FACIAL", PurchaseCategory4Code: "A020101",
					PurchaseCategory5Name: "MASQUE", PurchaseCategory5Code: "A02010101",
					Matnr: "552646", ArticleName: "Mask", BrandName: "Brand", TPSaleQuantity: 3, TPSaleAmount: 100,
					TPReturnSaleQuantity: 1, TPReturnSaleAmount: 10, TPGrossSaleQuantity: 2, TPGrossSaleAmount: 90,
				}},
			},
			"108": {
				TotalTransactionCount: &transactions108,
				Items: []rtasales.SaleItem{{
					PurchaseCategory1Name: "B-NON FOOD", PurchaseCategory2Name: "HOUSEHOLD",
					PurchaseCategory3Name: "CLEANING", PurchaseCategory4Name: "SURFACE", PurchaseCategory5Name: "WIPES",
					Matnr: "900001", ArticleName: "Wipes", TPSaleQuantity: 5, TPSaleAmount: 50,
					TPReturnSaleQuantity: 0, TPReturnSaleAmount: 5, TPGrossSaleQuantity: 5, TPGrossSaleAmount: 45,
				}},
			},
		},
		started: make(chan struct{}, 2),
		release: make(chan struct{}),
	}
	app, _, events := newTestApp(t, new(fakeEngine), fakeClients{byAccount: map[string]accountClient{"analysis-account": client}})
	profile, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		DisplayName: "Analysis", Account: "analysis-account", Password: "password", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	stores, err := app.ListSalesAnalysisStores(ProfileIDRequest{ProfileID: profile.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(stores) != 2 || stores[0].BusinessID != "107" || stores[1].BusinessID != "108" {
		t.Fatalf("unexpected sorted stores: %#v", stores)
	}

	type response struct {
		result SalesAnalysisResult
		err    error
	}
	done := make(chan response, 1)
	go func() {
		result, runErr := app.RunSalesAnalysis(SalesAnalysisRequest{
			ProfileID: profile.ID, StoreIDs: []string{"107", "108", "107"},
			From: "2026-08-15", To: "2026-08-15", Concurrency: 2,
		})
		done <- response{result: result, err: runErr}
	}()
	for range 2 {
		select {
		case <-client.started:
		case <-time.After(time.Second):
			t.Fatal("expected both store queries to start on separate logins")
		}
	}
	close(client.release)
	answer := <-done
	if answer.err != nil {
		t.Fatal(answer.err)
	}
	result := answer.result
	if !result.Complete || result.SelectedStores != 2 || result.SuccessfulStores != 2 || len(result.Items) != 0 {
		t.Fatalf("unexpected slim analysis result: %#v", result)
	}
	if len(result.Periods) != 1 || result.Periods[0].ItemCount != 2 || len(result.Periods[0].Items) != 0 {
		t.Fatalf("slim period should omit item rows: %#v", result.Periods)
	}
	packed, err := app.GetSalesAnalysisItems(SalesAnalysisItemsRequest{OperationID: result.OperationID, PeriodKey: "current"})
	if err != nil {
		t.Fatal(err)
	}
	result.Items = unpackSalesAnalysisItems(packed, result.Stores)
	if result.Totals.SaleAmount != 150 || result.Totals.ReturnAmount != 15 || result.Totals.NetSalesAmount != 135 {
		t.Fatalf("unexpected totals: %#v", result.Totals)
	}
	if result.Totals.TransactionCount != nil {
		t.Fatalf("article-only current period should omit Trend totals: %#v", result.Totals.TransactionCount)
	}
	if result.Items[0].Category1 != "A-HEALTH & BEAUTY" || result.Items[0].Category5 != "MASQUE" ||
		result.Items[0].Category4Code != "A020101" || result.Items[0].BrandName != "Brand" {
		t.Fatalf("category hierarchy was not preserved: %#v", result.Items[0])
	}
	client.mu.Lock()
	maxActive := client.maxActive
	queries := append([]rtasales.SalesQuery(nil), client.queries...)
	client.mu.Unlock()
	if maxActive != 2 || len(queries) != 2 {
		t.Fatalf("two logins should query two stores in parallel: max=%d queries=%d", maxActive, len(queries))
	}
	for _, query := range queries {
		if query.StartDate.Format("2006-01-02") != "2026-08-15" || query.EndDate.Format("2006-01-02") != "2026-08-15" {
			t.Fatalf("unexpected query range: %#v", query)
		}
		if !query.SkipTrend {
			t.Fatal("category analysis should not issue an additional Trend View request")
		}
	}
	events.mu.Lock()
	defer events.mu.Unlock()
	progressEvents := 0
	for _, event := range events.events {
		if event.name == salesAnalysisProgressEventName {
			progressEvents++
		}
	}
	if progressEvents != 3 {
		t.Fatalf("progress events=%d, want initial plus two stores", progressEvents)
	}
}

func TestSalesAnalysisQueriesReportPeriodsInParallelAndIncludesTrend(t *testing.T) {
	transactions := 8.0
	trend107, trend108 := 12.0, 22.0
	client := &salesAnalysisFakeClient{
		stores: []rtasales.Store{{BusinessID: "107", Label: "107 - First"}, {BusinessID: "108", Label: "108 - Second"}},
		results: map[string]*rtasales.SalesResult{
			"107": {
				TrendGrossSaleAmount: &trend107, TotalTransactionCount: &transactions,
				Items: []rtasales.SaleItem{{Matnr: "1", TPGrossSaleAmount: 10}},
				TrendDays: []rtasales.TrendDay{
					{Date: "2026-07-27", GrossSaleAmount: 5, TransactionCount: 2},
					{Date: "2026-08-03", GrossSaleAmount: 7, TransactionCount: 3},
				},
			},
			"108": {
				TrendGrossSaleAmount: &trend108, TotalTransactionCount: &transactions,
				Items: []rtasales.SaleItem{{Matnr: "2", TPGrossSaleAmount: 20}},
				TrendDays: []rtasales.TrendDay{
					{Date: "2026-07-27", GrossSaleAmount: 9, TransactionCount: 4},
					{Date: "2026-08-03", GrossSaleAmount: 13, TransactionCount: 5},
				},
			},
		},
		started: make(chan struct{}, 8),
		release: make(chan struct{}),
	}
	app, _, events := newTestApp(t, new(fakeEngine), fakeClients{byAccount: map[string]accountClient{"analysis-account": client}})
	profile, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		DisplayName: "Analysis", Account: "analysis-account", Password: "password", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	var result SalesAnalysisResult
	go func() {
		var runErr error
		result, runErr = app.RunSalesAnalysis(SalesAnalysisRequest{
			ProfileID: profile.ID, StoreIDs: []string{"107", "108"}, Concurrency: 4,
			Periods: []SalesAnalysisPeriodRequest{
				{Key: "current", Label: "Current", From: "2026-08-01", To: "2026-08-31", IncludeTrend: true},
				{Key: "yearAgo", Label: "Year ago", From: "2025-08-01", To: "2025-08-31", IncludeTrend: true},
			},
		})
		done <- runErr
	}()
	for range 2 {
		select {
		case <-client.started:
		case <-time.After(time.Second):
			t.Fatal("current-period article query did not start")
		}
	}
	select {
	case <-client.started:
		t.Fatal("follow-on periods started before the current article wave finished")
	case <-time.After(50 * time.Millisecond):
	}
	close(client.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !result.Pending || len(result.Periods) != 1 || result.Periods[0].Key != "current" {
		t.Fatalf("primary result should be current articles only: %#v", result)
	}
	if result.Periods[0].Totals.TransactionCount != nil {
		t.Fatalf("current article wave should omit Trend totals: %#v", result.Periods[0].Totals)
	}
	settled := waitSalesAnalysisSettled(t, app, result.OperationID)
	if len(settled.Periods) != 2 || settled.Periods[0].Key != "current" {
		t.Fatalf("unexpected period results: %#v", settled.Periods)
	}
	if settled.Periods[0].Totals.TransactionCount == nil || *settled.Periods[0].Totals.TransactionCount != 16 {
		t.Fatalf("unexpected period transactions: %#v", settled.Periods[0].Totals.TransactionCount)
	}
	if settled.Periods[0].Totals.TrendNetSalesAmount == nil || *settled.Periods[0].Totals.TrendNetSalesAmount != 34 {
		t.Fatalf("unexpected period Trend View net sales: %#v", settled.Periods[0].Totals.TrendNetSalesAmount)
	}
	if len(settled.Weeks) == 0 || settled.Weeks[0].From != "2026-07-27" || settled.Weeks[0].Totals.SalesTW != 14 {
		t.Fatalf("weekly fold missing or wrong: %#v", settled.Weeks)
	}
	client.mu.Lock()
	queries := append([]rtasales.SalesQuery(nil), client.queries...)
	client.mu.Unlock()
	articles, trends := 0, 0
	for _, query := range queries {
		if query.AllStores {
			trends++
			if !query.SkipArticle {
				t.Fatal("all-stores Trend View must skip Article View")
			}
			continue
		}
		articles++
		if !query.SkipTrend {
			t.Fatal("per-store report queries must skip Trend View")
		}
	}
	if articles != 4 || trends != 2 {
		t.Fatalf("articles=%d trends=%d, want 4 store reports and 2 all-store trends", articles, trends)
	}
	events.mu.Lock()
	defer events.mu.Unlock()
	progressEvents := 0
	for _, event := range events.events {
		if event.name == salesAnalysisProgressEventName {
			progressEvents++
		}
	}
	if progressEvents < 5 {
		t.Fatalf("progress events=%d, want initial plus follow-on work", progressEvents)
	}
}

func TestAccountQuerySessionCountCapsExtraLogins(t *testing.T) {
	if got := accountQuerySessionCount(40, 160); got != 40 {
		t.Fatalf("40 stores at 160 concurrency = %d, want 40", got)
	}
	if got := accountQuerySessionCount(200, 160); got != maxAccountQuerySessions {
		t.Fatalf("200 stores at 160 concurrency = %d, want %d", got, maxAccountQuerySessions)
	}
	if got := accountQuerySessionCount(2, 160); got != 2 {
		t.Fatalf("two stores = %d, want 2", got)
	}
	if got := accountQuerySessionCount(16, 1); got != 1 {
		t.Fatalf("concurrency 1 = %d, want 1", got)
	}
	if got := accountQuerySessionCount(0, 160); got != 1 {
		t.Fatalf("no stores = %d, want 1", got)
	}
}

func TestSalesAnalysisOverlapsFollowOnBeforeAllCurrentFinish(t *testing.T) {
	holdB := make(chan struct{})
	previousA := make(chan struct{})
	var previousOnce sync.Once
	stores := []rtasales.Store{
		{BusinessID: "S01", Label: "S01"},
		{BusinessID: "S02", Label: "S02"},
	}
	client := &salesAnalysisFakeClient{
		stores:  stores,
		results: map[string]*rtasales.SalesResult{"S01": {}, "S02": {}},
		hold: func(query rtasales.SalesQuery) <-chan struct{} {
			day := query.StartDate.Format("2006-01-02")
			if query.BusinessStoreID == "S02" && day == "2026-08-15" {
				return holdB
			}
			if query.BusinessStoreID == "S01" && day == "2026-07-15" {
				previousOnce.Do(func() { close(previousA) })
			}
			return nil
		},
	}
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{byAccount: map[string]accountClient{"analysis-account": client}})
	profile, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		DisplayName: "Analysis", Account: "analysis-account", Password: "password", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 1)
	go func() {
		_, runErr := app.RunSalesAnalysis(SalesAnalysisRequest{
			ProfileID: profile.ID, StoreIDs: []string{"S01", "S02"}, Concurrency: 160,
			Periods: []SalesAnalysisPeriodRequest{
				{Key: "current", Label: "Current", From: "2026-08-15", To: "2026-08-15"},
				{Key: "previous", Label: "Previous", From: "2026-07-15", To: "2026-07-15"},
			},
		})
		errs <- runErr
	}()
	select {
	case <-previousA:
	case <-time.After(2 * time.Second):
		t.Fatal("store S01 previous period did not start before S02 current finished")
	}
	close(holdB)
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
}

func TestSalesAnalysisSerializesPerProfileAtDefaultAndMaximumConcurrency(t *testing.T) {
	periods := []SalesAnalysisPeriodRequest{
		{Key: "current", Label: "Current", From: "2026-08-15", To: "2026-08-15"},
		{Key: "previous", Label: "Previous", From: "2026-07-15", To: "2026-07-15"},
		{Key: "previous2", Label: "Previous 2", From: "2026-06-15", To: "2026-06-15"},
		{Key: "yearAgo", Label: "Year ago", From: "2025-08-15", To: "2025-08-15"},
	}
	for _, test := range []struct {
		name        string
		concurrency int
		storeCount  int
		wantActive  int
	}{
		{name: "default opens one login per store", concurrency: 0, storeCount: 40, wantActive: 40},
		{name: "concurrency caps sessions below store count", concurrency: 3, storeCount: 5, wantActive: 3},
		{name: "one store stays on one login", concurrency: 160, storeCount: 1, wantActive: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			stores := make([]rtasales.Store, 0, test.storeCount)
			results := make(map[string]*rtasales.SalesResult, test.storeCount)
			storeIDs := make([]string, 0, test.storeCount)
			for index := 0; index < test.storeCount; index++ {
				storeID := fmt.Sprintf("S%02d", index+1)
				stores = append(stores, rtasales.Store{BusinessID: storeID, Label: storeID})
				storeIDs = append(storeIDs, storeID)
				results[storeID] = &rtasales.SalesResult{}
			}
			client := &salesAnalysisFakeClient{
				stores: stores, results: results, started: make(chan struct{}, 256), release: make(chan struct{}),
			}
			app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{byAccount: map[string]accountClient{"analysis-account": client}})
			profile, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
				DisplayName: "Analysis", Account: "analysis-account", Password: "password", Enabled: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan SalesAnalysisResult, 1)
			errs := make(chan error, 1)
			go func() {
				result, runErr := app.RunSalesAnalysis(SalesAnalysisRequest{
					ProfileID: profile.ID, StoreIDs: storeIDs, Periods: periods, Concurrency: test.concurrency,
				})
				errs <- runErr
				done <- result
			}()
			for range test.wantActive {
				select {
				case <-client.started:
				case <-time.After(2 * time.Second):
					t.Fatalf("only part of the expected %d concurrent tasks started", test.wantActive)
				}
			}
			select {
			case <-client.started:
				t.Fatal("account opened more query sessions than the login cap")
			case <-time.After(50 * time.Millisecond):
			}
			close(client.release)
			if err := <-errs; err != nil {
				t.Fatal(err)
			}
			primary := <-done
			if !primary.Pending {
				t.Fatal("expected remaining periods to load in the background")
			}
			waitSalesAnalysisSettled(t, app, primary.OperationID)
			client.mu.Lock()
			maxActive := client.maxActive
			queryCount := len(client.queries)
			client.mu.Unlock()
			if maxActive != test.wantActive {
				t.Fatalf("maxActive=%d, want %d", maxActive, test.wantActive)
			}
			trendJobs := 0
			for _, period := range periods {
				if period.IncludeTrend {
					trendJobs++
				}
			}
			wantQueries := test.storeCount*len(periods) + trendJobs
			if queryCount != wantQueries {
				t.Fatalf("queries=%d, want %d articles plus %d all-store trends", queryCount, test.storeCount*len(periods), trendJobs)
			}
		})
	}
}

func TestSalesAnalysisOneAccountManyStores429IsNotPermission(t *testing.T) {
	const storeCount = 16
	failID := "S03"
	stores := make([]rtasales.Store, storeCount)
	results := make(map[string]*rtasales.SalesResult, storeCount)
	storeIDs := make([]string, storeCount)
	transactions := 4.0
	for index := 0; index < storeCount; index++ {
		storeID := "S" + twoDigit(index+1)
		stores[index] = rtasales.Store{BusinessID: storeID, Label: storeID + " - Store"}
		storeIDs[index] = storeID
		results[storeID] = &rtasales.SalesResult{
			TotalTransactionCount: &transactions,
			Items: []rtasales.SaleItem{{
				Matnr: "ITEM", ArticleName: "Item", TPSaleQuantity: 1, TPSaleAmount: 10,
				TPGrossSaleQuantity: 1, TPGrossSaleAmount: 10,
			}},
		}
	}
	client := &salesAnalysisFakeClient{
		stores:  stores,
		results: results,
		failOn: func(_ int, query rtasales.SalesQuery) error {
			if query.BusinessStoreID == failID {
				return &rtasales.UpstreamError{Operation: "sales", StatusCode: 429, Body: "too many requests"}
			}
			return nil
		},
	}
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{byAccount: map[string]accountClient{"analysis-account": client}})
	app.salesAnalysisBackoff = func(context.Context, time.Duration) error { return nil }
	profile, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		DisplayName: "Analysis", Account: "analysis-account", Password: "password", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := app.ListSalesAnalysisStores(ProfileIDRequest{ProfileID: profile.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != storeCount {
		t.Fatalf("listed stores=%d, want %d", len(listed), storeCount)
	}
	result, err := app.RunSalesAnalysis(SalesAnalysisRequest{
		ProfileID: profile.ID, StoreIDs: storeIDs, From: "2026-08-15", To: "2026-08-15",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SelectedStores != storeCount || result.SuccessfulStores != storeCount-1 {
		t.Fatalf("selected=%d successful=%d, want %d/%d", result.SelectedStores, result.SuccessfulStores, storeCount, storeCount-1)
	}
	if len(result.Issues) != 1 {
		t.Fatalf("issues=%d, want 1: %#v", len(result.Issues), result.Issues)
	}
	if !strings.Contains(result.Issues[0].Message, "429") {
		t.Fatalf("issue did not keep the 429: %s", result.Issues[0].Message)
	}
	if permissionIssue(result.Issues[0].Message) {
		t.Fatalf("429 classified as permission: %s", result.Issues[0].Message)
	}
	if result.Complete {
		t.Fatal("429 should leave the run incomplete")
	}
}

func TestSalesAnalysisRejectsUnauthorizedStoreBeforeQuery(t *testing.T) {
	client := &salesAnalysisFakeClient{
		stores:  []rtasales.Store{{BusinessID: "107", Label: "107 - First"}},
		results: map[string]*rtasales.SalesResult{},
	}
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{byAccount: map[string]accountClient{"analysis-account": client}})
	profile, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		DisplayName: "Analysis", Account: "analysis-account", Password: "password", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.RunSalesAnalysis(SalesAnalysisRequest{
		ProfileID: profile.ID, StoreIDs: []string{"999"}, From: "2026-08-15", To: "2026-08-15",
	})
	if err == nil {
		t.Fatal("expected unauthorized store rejection")
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.queries) != 0 {
		t.Fatalf("unauthorized store reached Sales: %#v", client.queries)
	}
}

func TestCancelSalesAnalysisReleasesOperationLock(t *testing.T) {
	client := &salesAnalysisFakeClient{
		stores:  []rtasales.Store{{BusinessID: "107", Label: "107 - First"}},
		results: map[string]*rtasales.SalesResult{},
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{byAccount: map[string]accountClient{"analysis-account": client}})
	profile, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		DisplayName: "Analysis", Account: "analysis-account", Password: "password", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, runErr := app.RunSalesAnalysis(SalesAnalysisRequest{
			ProfileID: profile.ID, StoreIDs: []string{"107"}, From: "2026-08-15", To: "2026-08-15",
		})
		done <- runErr
	}()
	select {
	case <-client.started:
	case <-time.After(time.Second):
		t.Fatal("sales analysis did not start")
	}
	app.operationMu.Lock()
	operationID := app.salesAnalysisID
	app.operationMu.Unlock()
	if err := app.CancelSalesAnalysis(OperationRequest{OperationID: operationID}); err != nil {
		t.Fatal(err)
	}
	select {
	case runErr := <-done:
		if !errors.Is(runErr, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", runErr)
		}
	case <-time.After(time.Second):
		t.Fatal("sales analysis did not stop after cancellation")
	}
	if _, err := app.Enable(EnableProfileRequest{ProfileID: profile.ID, Enabled: false}); err != nil {
		t.Fatalf("operation lock was not released: %v", err)
	}
}

func TestSalesAnalysisQueriesTwoProfilesAtTheSameTime(t *testing.T) {
	transactions := 8.0
	clientA := &salesAnalysisFakeClient{
		stores: []rtasales.Store{{BusinessID: "107", Label: "107 - Central"}},
		results: map[string]*rtasales.SalesResult{
			"107": {TotalTransactionCount: &transactions, Items: []rtasales.SaleItem{{Matnr: "A1", TPGrossSaleAmount: 20}}},
		},
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	clientB := &salesAnalysisFakeClient{
		stores: []rtasales.Store{
			{BusinessID: "107", Label: "107 - Other"},
			{BusinessID: "108", Label: "108 - Harbour"},
		},
		results: map[string]*rtasales.SalesResult{
			"108": {TotalTransactionCount: &transactions, Items: []rtasales.SaleItem{{Matnr: "B1", TPGrossSaleAmount: 30}}},
		},
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{byAccount: map[string]accountClient{
		"account-a": clientA,
		"account-b": clientB,
	}})
	profileA, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		DisplayName: "North", Account: "account-a", Password: "password", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		DisplayName: "South", Account: "account-b", Password: "password", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	stores, err := app.ListSalesAnalysisStores(ProfileIDRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(stores) != 2 || stores[0].BusinessID != "107" || stores[0].ProfileID != profileA.ID || stores[1].BusinessID != "108" {
		t.Fatalf("union stores=%#v", stores)
	}

	type response struct {
		result SalesAnalysisResult
		err    error
	}
	done := make(chan response, 1)
	go func() {
		result, runErr := app.RunSalesAnalysis(SalesAnalysisRequest{
			StoreIDs: []string{"107", "108"}, From: "2026-08-15", To: "2026-08-15", Concurrency: 2,
		})
		done <- response{result: result, err: runErr}
	}()
	for _, started := range []*salesAnalysisFakeClient{clientA, clientB} {
		select {
		case <-started.started:
		case <-time.After(time.Second):
			t.Fatal("both accounts did not start querying at the same time")
		}
	}
	close(clientA.release)
	close(clientB.release)
	answer := <-done
	if answer.err != nil {
		t.Fatal(answer.err)
	}
	if answer.result.SuccessfulStores != 2 {
		t.Fatalf("successful=%d, want 2", answer.result.SuccessfulStores)
	}
	if len(clientA.queries) != 1 || clientA.queries[0].BusinessStoreID != "107" {
		t.Fatalf("account A queries=%#v", clientA.queries)
	}
	if len(clientB.queries) != 1 || clientB.queries[0].BusinessStoreID != "108" {
		t.Fatalf("account B queries=%#v", clientB.queries)
	}
}

func TestSalesAnalysisRateLimitPausesOnlyTheAffectedAccount(t *testing.T) {
	clientA := &salesAnalysisFakeClient{
		stores: []rtasales.Store{{BusinessID: "A1", Label: "A1"}, {BusinessID: "A2", Label: "A2"}},
		results: map[string]*rtasales.SalesResult{
			"A1": {Items: []rtasales.SaleItem{{Matnr: "A1", TPGrossSaleAmount: 10}}},
			"A2": {Items: []rtasales.SaleItem{{Matnr: "A2", TPGrossSaleAmount: 20}}},
		},
		failOn: func(call int, _ rtasales.SalesQuery) error {
			if call == 1 {
				return &rtasales.UpstreamError{Operation: "sales", StatusCode: 429, Body: "too many requests"}
			}
			return nil
		},
	}
	clientB := &salesAnalysisFakeClient{
		stores:  []rtasales.Store{{BusinessID: "B1", Label: "B1"}},
		results: map[string]*rtasales.SalesResult{"B1": {Items: []rtasales.SaleItem{{Matnr: "B1", TPGrossSaleAmount: 30}}}},
		started: make(chan struct{}, 1),
	}
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{byAccount: map[string]accountClient{
		"account-a": clientA,
		"account-b": clientB,
	}})
	if _, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		DisplayName: "North", Account: "account-a", Password: "password", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		DisplayName: "South", Account: "account-b", Password: "password", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	paused := make(chan time.Duration, 1)
	resume := make(chan struct{})
	app.salesAnalysisBackoff = func(ctx context.Context, delay time.Duration) error {
		paused <- delay
		select {
		case <-resume:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	type response struct {
		result SalesAnalysisResult
		err    error
	}
	done := make(chan response, 1)
	go func() {
		result, runErr := app.RunSalesAnalysis(SalesAnalysisRequest{
			StoreIDs: []string{"A1", "A2", "B1"}, From: "2026-08-15", To: "2026-08-15", Concurrency: 160,
		})
		done <- response{result: result, err: runErr}
	}()

	select {
	case delay := <-paused:
		if delay != time.Second {
			t.Fatalf("first rate-limit pause=%s, want 1s", delay)
		}
	case <-time.After(time.Second):
		t.Fatal("rate-limited account did not enter backoff")
	}
	select {
	case <-clientB.started:
	case <-time.After(time.Second):
		t.Fatal("second account did not continue while the first account was paused")
	}
	clientA.mu.Lock()
	queriesWhilePaused := len(clientA.queries)
	clientA.mu.Unlock()
	if queriesWhilePaused < 1 || queriesWhilePaused > 2 {
		t.Fatalf("affected account ran %d queries before its pause was released", queriesWhilePaused)
	}
	close(resume)
	answer := <-done
	if answer.err != nil {
		t.Fatal(answer.err)
	}
	if !answer.result.Complete || answer.result.SuccessfulStores != 3 {
		t.Fatalf("rate-limit retry did not recover: %#v", answer.result)
	}
	clientA.mu.Lock()
	queriesA, maxActiveA := len(clientA.queries), clientA.maxActive
	clientA.mu.Unlock()
	if queriesA < 3 || maxActiveA < 1 || maxActiveA > 2 {
		t.Fatalf("account A queries=%d maxActive=%d, want retries with at most two logins", queriesA, maxActiveA)
	}
}

func TestSalesAnalysisSubsetTrendDoesNotIncludeUnselectedStores(t *testing.T) {
	transactions107, transactions108 := 10.0, 12.0
	trend107, trend108 := 12.0, 22.0
	client := &salesAnalysisFakeClient{
		stores: []rtasales.Store{
			{BusinessID: "107", Label: "107 - First"},
			{BusinessID: "108", Label: "108 - Second"},
		},
		results: map[string]*rtasales.SalesResult{
			"107": {
				TrendGrossSaleAmount: &trend107, TotalTransactionCount: &transactions107,
				Items: []rtasales.SaleItem{{Matnr: "1", TPGrossSaleAmount: 10}},
				TrendDays: []rtasales.TrendDay{
					{Date: "2026-08-15", GrossSaleAmount: 12, TransactionCount: 10},
				},
			},
			"108": {
				TrendGrossSaleAmount: &trend108, TotalTransactionCount: &transactions108,
				Items: []rtasales.SaleItem{{Matnr: "2", TPGrossSaleAmount: 20}},
				TrendDays: []rtasales.TrendDay{
					{Date: "2026-08-15", GrossSaleAmount: 22, TransactionCount: 12},
				},
			},
		},
	}
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{byAccount: map[string]accountClient{"analysis-account": client}})
	profile, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		DisplayName: "Analysis", Account: "analysis-account", Password: "password", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := app.RunSalesAnalysis(SalesAnalysisRequest{
		ProfileID: profile.ID, StoreIDs: []string{"107"}, Concurrency: 1,
		Periods: []SalesAnalysisPeriodRequest{
			{Key: "current", Label: "Current", From: "2026-08-15", To: "2026-08-15", IncludeTrend: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	settled := waitSalesAnalysisSettled(t, app, result.OperationID)
	if settled.Periods[0].Totals.TrendNetSalesAmount == nil || *settled.Periods[0].Totals.TrendNetSalesAmount != trend107 {
		t.Fatalf("subset trend sales=%v, want %v", settled.Periods[0].Totals.TrendNetSalesAmount, trend107)
	}
	if settled.Periods[0].Totals.TransactionCount == nil || *settled.Periods[0].Totals.TransactionCount != transactions107 {
		t.Fatalf("subset trend tickets=%v, want %v", settled.Periods[0].Totals.TransactionCount, transactions107)
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	for _, query := range client.queries {
		if query.AllStores {
			t.Fatalf("subset analysis used all-stores Trend View: %#v", query)
		}
		if query.SkipArticle && query.BusinessStoreID != "107" {
			t.Fatalf("subset trend queried unselected store: %#v", query)
		}
	}
}
