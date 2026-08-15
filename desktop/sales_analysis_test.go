package desktop

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	rtasales "github.com/Miku0139oao/rta-sales-client-go"
)

type salesAnalysisFakeClient struct {
	stores  []rtasales.Store
	results map[string]*rtasales.SalesResult

	mu        sync.Mutex
	queries   []rtasales.SalesQuery
	active    int
	maxActive int
	started   chan struct{}
	release   chan struct{}
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
	if c.release != nil {
		select {
		case <-c.release:
		case <-ctx.Done():
			c.mu.Lock()
			c.active--
			c.mu.Unlock()
			return nil, ctx.Err()
		}
	}
	c.mu.Lock()
	c.active--
	c.mu.Unlock()
	result := c.results[query.BusinessStoreID]
	if result == nil {
		return nil, errors.New("missing fake sales result")
	}
	return result, nil
}

func TestSalesAnalysisUsesOneProfileForParallelStoresAndPreservesCategories(t *testing.T) {
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
			t.Fatal("store queries did not start in parallel")
		}
	}
	close(client.release)
	answer := <-done
	if answer.err != nil {
		t.Fatal(answer.err)
	}
	result := answer.result
	if !result.Complete || result.SelectedStores != 2 || result.SuccessfulStores != 2 || len(result.Items) != 2 {
		t.Fatalf("unexpected analysis result: %#v", result)
	}
	if result.Totals.SaleAmount != 150 || result.Totals.ReturnAmount != 15 || result.Totals.NetSalesAmount != 135 {
		t.Fatalf("unexpected totals: %#v", result.Totals)
	}
	if result.Totals.TransactionCount == nil || *result.Totals.TransactionCount != 22 {
		t.Fatalf("unexpected transaction total: %#v", result.Totals.TransactionCount)
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
		t.Fatalf("parallel queries max=%d queries=%d", maxActive, len(queries))
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
			"107": {TrendGrossSaleAmount: &trend107, TotalTransactionCount: &transactions, Items: []rtasales.SaleItem{{Matnr: "1", TPGrossSaleAmount: 10}}},
			"108": {TrendGrossSaleAmount: &trend108, TotalTransactionCount: &transactions, Items: []rtasales.SaleItem{{Matnr: "2", TPGrossSaleAmount: 20}}},
		},
		started: make(chan struct{}, 4),
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
	for range 4 {
		select {
		case <-client.started:
		case <-time.After(time.Second):
			t.Fatal("period/store queries did not start in parallel")
		}
	}
	close(client.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(result.Periods) != 2 || result.Periods[0].Key != "current" || result.Periods[1].Key != "yearAgo" {
		t.Fatalf("unexpected period results: %#v", result.Periods)
	}
	if result.Periods[0].Totals.TransactionCount == nil || *result.Periods[0].Totals.TransactionCount != 16 {
		t.Fatalf("unexpected period transactions: %#v", result.Periods[0].Totals.TransactionCount)
	}
	if result.Periods[0].Totals.TrendNetSalesAmount == nil || *result.Periods[0].Totals.TrendNetSalesAmount != 34 {
		t.Fatalf("unexpected period Trend View net sales: %#v", result.Periods[0].Totals.TrendNetSalesAmount)
	}
	client.mu.Lock()
	queries := append([]rtasales.SalesQuery(nil), client.queries...)
	maxActive := client.maxActive
	client.mu.Unlock()
	if len(queries) != 4 || maxActive != 4 {
		t.Fatalf("queries=%d maxActive=%d, want 4", len(queries), maxActive)
	}
	for _, query := range queries {
		if query.SkipTrend {
			t.Fatal("report period requested Trend View totals")
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
	if progressEvents != 5 {
		t.Fatalf("progress events=%d, want initial plus four tasks", progressEvents)
	}
}

func TestSalesAnalysisSupportsDefault16AndMaximum32ParallelTasksPerProfile(t *testing.T) {
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
		{name: "default 16", concurrency: 0, storeCount: 5, wantActive: 16},
		{name: "maximum 32", concurrency: 32, storeCount: 8, wantActive: 32},
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
			taskCount := test.storeCount * len(periods)
			client := &salesAnalysisFakeClient{
				stores: stores, results: results, started: make(chan struct{}, taskCount), release: make(chan struct{}),
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
					ProfileID: profile.ID, StoreIDs: storeIDs, Periods: periods, Concurrency: test.concurrency,
				})
				done <- runErr
			}()
			for range test.wantActive {
				select {
				case <-client.started:
				case <-time.After(2 * time.Second):
					t.Fatalf("only part of the expected %d concurrent tasks started", test.wantActive)
				}
			}
			close(client.release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			client.mu.Lock()
			maxActive := client.maxActive
			queryCount := len(client.queries)
			client.mu.Unlock()
			if maxActive != test.wantActive || queryCount != taskCount {
				t.Fatalf("maxActive=%d queries=%d, want %d and %d", maxActive, queryCount, test.wantActive, taskCount)
			}
		})
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
