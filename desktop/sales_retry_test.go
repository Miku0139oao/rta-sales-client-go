package desktop

import (
	"errors"
	rtasales "github.com/Miku0139oao/rta-sales-client-go/rtasales"
	"testing"
)

func TestRetrySalesAnalysisPreservesSuccessfulJobs(t *testing.T) {
	client := reportCacheFakeClient()
	client.stores = append(client.stores, rtasales.Store{BusinessID: "108", Label: "Second"})
	client.results["108"] = client.results["107"]
	failed := false
	client.failOn = func(_ int, q rtasales.SalesQuery) error {
		if q.BusinessStoreID == "108" && !failed {
			failed = true
			return errors.New("temporary upstream failure")
		}
		return nil
	}
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{byAccount: map[string]accountClient{"analysis-account": client}})
	profile, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{DisplayName: "Analysis", Account: "analysis-account", Password: "password", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	result, err := app.RunSalesAnalysis(SalesAnalysisRequest{ProfileID: profile.ID, StoreIDs: []string{"107", "108"}, From: "2026-08-15", To: "2026-08-15", Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Complete || len(result.Issues) != 1 {
		t.Fatalf("expected partial report: %#v", result)
	}
	retried, err := app.RetrySalesAnalysis(OperationRequest{OperationID: result.OperationID})
	if err != nil {
		t.Fatal(err)
	}
	if !retried.Complete || retried.Totals.NetSalesAmount != 200 {
		t.Fatalf("incorrect merged result: %#v", retried)
	}
	counts := map[string]int{}
	for _, query := range client.queries {
		counts[query.BusinessStoreID]++
	}
	if counts["107"] != 1 || counts["108"] != 2 {
		t.Fatalf("successful jobs repeated: %v", counts)
	}
	if _, err := app.RetrySalesAnalysis(OperationRequest{OperationID: "expired"}); err == nil {
		t.Fatal("accepted expired retry")
	}
}
