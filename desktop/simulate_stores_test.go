package desktop

import (
	"math"
	"strings"
	"testing"

	rtasales "github.com/Miku0139oao/rta-sales-client-go"
)

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
	if queryCount != 1 {
		t.Fatalf("inner queries=%d, want 1 cached real-store fetch", queryCount)
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
