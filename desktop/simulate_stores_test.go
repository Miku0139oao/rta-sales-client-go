package desktop

import (
	"context"
	"math"
	"strings"
	"testing"

	rtasales "github.com/Miku0139oao/rta-sales-client-go"
)

func TestSimulatingClientHitsInnerSalesForEveryClone(t *testing.T) {
	client := &salesAnalysisFakeClient{
		stores: []rtasales.Store{{BusinessID: "107", Label: "107"}},
		results: map[string]*rtasales.SalesResult{
			"107": {Items: []rtasales.SaleItem{{Matnr: "A1", TPGrossSaleAmount: 10}}},
		},
	}
	sim, ok := maybeSimulateClient(client, 3).(*simulatingClient)
	if !ok {
		t.Fatal("expected a simulating client")
	}
	for _, storeID := range []string{"107", "107~sim02", "107~sim03"} {
		if _, err := sim.Sales(context.Background(), rtasales.SalesQuery{BusinessStoreID: storeID}); err != nil {
			t.Fatal(err)
		}
	}
	if len(client.queries) != 3 {
		t.Fatalf("inner queries=%d, want 3 live real-store fetches", len(client.queries))
	}
	for _, query := range client.queries {
		if query.BusinessStoreID != "107" {
			t.Fatalf("inner query used %q, want 107", query.BusinessStoreID)
		}
	}
}

func TestExpandSimulatedStoresKeepsRealStoresThenAddsClones(t *testing.T) {
	stores := []rtasales.Store{{BusinessID: "107", Label: "107 - Central"}}
	expanded := expandSimulatedStores(stores, 16)
	if len(expanded) != 16 {
		t.Fatalf("stores=%d, want 16", len(expanded))
	}
	if expanded[0] != stores[0] {
		t.Fatalf("first store=%+v, want the real store", expanded[0])
	}
	if expanded[1].BusinessID != "107~sim02" || !strings.Contains(expanded[1].Label, "模擬 02") {
		t.Fatalf("first clone=%+v", expanded[1])
	}
	if expanded[15].BusinessID != "107~sim16" {
		t.Fatalf("last clone=%q", expanded[15].BusinessID)
	}
}

func TestSalesAnalysisSimulatesSixteenStoresFromOneAuthorizedStore(t *testing.T) {
	transactions := 20.0
	trend := 400.0
	client := &salesAnalysisFakeClient{
		stores: []rtasales.Store{{BusinessID: "107", Label: "107 - Central"}},
		results: map[string]*rtasales.SalesResult{
			"107": {
				TotalTransactionCount: &transactions,
				TrendGrossSaleAmount:  &trend,
				Items: []rtasales.SaleItem{{
					PurchaseCategory1Name: "HEALTH & BEAUTY", Matnr: "552646", ArticleName: "Mask",
					TPSaleQuantity: 4, TPSaleAmount: 200, TPGrossSaleQuantity: 4, TPGrossSaleAmount: 200,
				}},
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

	stores, err := app.ListSalesAnalysisStores(ProfileIDRequest{ProfileID: profile.ID, SimulateStoreCount: 16})
	if err != nil {
		t.Fatal(err)
	}
	if len(stores) != 16 {
		t.Fatalf("listed stores=%d, want 16", len(stores))
	}
	storeIDs := make([]string, 0, len(stores))
	for _, store := range stores {
		storeIDs = append(storeIDs, store.BusinessID)
	}

	result, err := app.RunSalesAnalysis(SalesAnalysisRequest{
		ProfileID: profile.ID, StoreIDs: storeIDs, From: "2026-08-15", To: "2026-08-15",
		SimulateStoreCount: 16,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SelectedStores != 16 || result.SuccessfulStores != 16 {
		t.Fatalf("selected=%d successful=%d, want 16/16", result.SelectedStores, result.SuccessfulStores)
	}
	packed, err := app.GetSalesAnalysisItems(SalesAnalysisItemsRequest{OperationID: result.OperationID, PeriodKey: "current"})
	if err != nil {
		t.Fatal(err)
	}
	result.Items = unpackSalesAnalysisItems(packed, result.Stores)
	if len(result.Items) != 16 {
		t.Fatalf("items=%d, want 16", len(result.Items))
	}
	client.mu.Lock()
	queryCount := len(client.queries)
	for _, query := range client.queries {
		if query.BusinessStoreID != "107" {
			t.Fatalf("inner query used %q, want the real store", query.BusinessStoreID)
		}
	}
	client.mu.Unlock()
	if queryCount != 16 {
		t.Fatalf("inner queries=%d, want 16 live real-store fetches", queryCount)
	}

	amounts := map[string]float64{}
	for _, item := range result.Items {
		amounts[item.StoreID] = item.NetSalesAmount
	}
	wantClone := 200 * simulatedStoreScale(2)
	if amounts["107"] != 200 || math.Abs(amounts["107~sim02"]-wantClone) > 0.001 {
		t.Fatalf("real=%.2f clone=%.2f, want 200 and %.2f", amounts["107"], amounts["107~sim02"], wantClone)
	}
}

func TestOneAccountSixteenStoresFivePeriodsHitsAPIAndKeeps429OnThatAccount(t *testing.T) {
	transactions := 20.0
	client := &salesAnalysisFakeClient{
		stores: []rtasales.Store{{BusinessID: "107", Label: "107 - Central"}},
		results: map[string]*rtasales.SalesResult{
			"107": {
				TotalTransactionCount: &transactions,
				Items: []rtasales.SaleItem{{
					PurchaseCategory2Name: "BEAUTY CARE", PurchaseCategory2Code: "A02",
					Matnr: "552646", ArticleName: "Mask",
					TPSaleQuantity: 4, TPSaleAmount: 200, TPGrossSaleQuantity: 4, TPGrossSaleAmount: 200,
				}},
			},
		},
		failOn: func(call int, _ rtasales.SalesQuery) error {
			if call%10 == 0 {
				return &rtasales.UpstreamError{Operation: "sales", StatusCode: 429, Body: "too many requests"}
			}
			return nil
		},
	}
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{byAccount: map[string]accountClient{"analysis-account": client}})
	profile, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		DisplayName: "Production", Account: "analysis-account", Password: "password", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	stores, err := app.ListSalesAnalysisStores(ProfileIDRequest{ProfileID: profile.ID, SimulateStoreCount: 16})
	if err != nil {
		t.Fatal(err)
	}
	storeIDs := make([]string, 0, len(stores))
	for _, store := range stores {
		storeIDs = append(storeIDs, store.BusinessID)
	}
	periods := []SalesAnalysisPeriodRequest{
		{Key: "current", Label: "本期", From: "2026-08-01", To: "2026-08-16", IncludeTrend: true},
		{Key: "previous", Label: "上期", From: "2026-07-01", To: "2026-07-16", IncludeTrend: true},
		{Key: "previous2", Label: "前期", From: "2026-06-01", To: "2026-06-16", IncludeTrend: true},
		{Key: "yearAgo", Label: "去年同期", From: "2025-08-01", To: "2025-08-16", IncludeTrend: true},
		{Key: "yearAgoNext", Label: "去年下月", From: "2025-09-01", To: "2025-09-30", IncludeTrend: false},
	}

	result, err := app.RunSalesAnalysis(SalesAnalysisRequest{
		ProfileID: profile.ID, StoreIDs: storeIDs, Periods: periods,
		Concurrency: 32, SimulateStoreCount: 16,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SelectedStores != 16 {
		t.Fatalf("selected=%d, want 16", result.SelectedStores)
	}
	if len(client.queries) != 80 {
		t.Fatalf("inner queries=%d, want 80 live hits on one account", len(client.queries))
	}
	for _, query := range client.queries {
		if query.BusinessStoreID != "107" {
			t.Fatalf("query used %q, want the single authorized store", query.BusinessStoreID)
		}
	}
	if len(result.Issues) != 8 {
		t.Fatalf("issues=%d, want 8 rate-limit failures", len(result.Issues))
	}
	for _, issue := range result.Issues {
		if !strings.Contains(issue.Message, "429") {
			t.Fatalf("issue did not keep the 429: %s", issue.Message)
		}
	}
	if result.Complete {
		t.Fatal("429 should leave the overall run incomplete")
	}
	incompletePeriods := 0
	for _, period := range result.Periods {
		if !period.Complete {
			incompletePeriods++
		}
	}
	if incompletePeriods == 0 {
		t.Fatalf("429 did not mark any period incomplete: %#v", result.Periods)
	}

	memo, err := app.GetSalesAnalysisReportMemo(SalesAnalysisReportMemoRequest{
		OperationID: result.OperationID, CategoryLevel: "category2", Mode: "blacklist",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(memo.Periods) != 5 {
		t.Fatalf("memo periods=%d, want 5", len(memo.Periods))
	}
	current := memo.Periods[0]
	if len(current.TopAmount) == 0 || current.TopAmount[0].Code != "552646" {
		t.Fatalf("combined memo missing ranked sales: %#v", current)
	}
	if len(result.Stores) == 0 {
		t.Fatal("expected at least one successful store for a single-store memo")
	}
	one, err := app.GetSalesAnalysisReportMemo(SalesAnalysisReportMemoRequest{
		OperationID: result.OperationID, StoreID: result.Stores[0].BusinessID, CategoryLevel: "category2", Mode: "blacklist",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(one.Periods[0].TopAmount) == 0 {
		t.Fatal("single-store memo was empty")
	}
}
