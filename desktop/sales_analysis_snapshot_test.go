package desktop

import (
	"bytes"
	"encoding/json"
	"testing"

	rtasales "github.com/Miku0139oao/rta-sales-client-go/rtasales"
)

func TestAttachTrendOutcomesPreservesArticleGrid(t *testing.T) {
	article := make([]storeOutcome, 1, 4)
	article[0].result = &rtasales.SalesResult{}
	original := [][]storeOutcome{article}
	trend := storeOutcome{result: &rtasales.SalesResult{}}
	combined := attachTrendOutcomes(original, []storeOutcome{trend})
	if len(original[0]) != 1 || len(combined[0]) != 2 {
		t.Fatal("trend attachment modified article headers")
	}
	combined[0][0] = storeOutcome{}
	if original[0][0].result == nil {
		t.Fatal("trend attachment reused article backing storage")
	}
}

func TestSalesAnalysisUpdateOwnsMutableSlices(t *testing.T) {
	original := SalesAnalysisResult{Issues: []SalesAnalysisIssue{{Message: "original"}}, Periods: []SalesAnalysisPeriodResult{{Key: "current", Issues: []SalesAnalysisIssue{{Message: "period original"}}}}}
	update := salesAnalysisForUpdate(original)
	update.Issues[0].Message = "changed"
	update.Periods[0].Issues[0].Message = "changed"
	update.Periods[0].Key = "changed"
	if original.Issues[0].Message != "original" || original.Periods[0].Issues[0].Message != "period original" || original.Periods[0].Key != "current" {
		t.Fatal("update mutated a published snapshot")
	}
}

func TestSalesAnalysisPublishedSnapshotRemainsStableDuringSupplement(t *testing.T) {
	release := make(chan struct{})
	transactions, trend := 5.0, 20.0
	client := &salesAnalysisFakeClient{
		stores:  []rtasales.Store{{BusinessID: "107", Label: "Synthetic"}},
		results: map[string]*rtasales.SalesResult{"107": {TotalTransactionCount: &transactions, TrendGrossSaleAmount: &trend, Items: []rtasales.SaleItem{{Matnr: "001", TPSaleAmount: 10, TPGrossSaleAmount: 10}}}},
		hold: func(query rtasales.SalesQuery) <-chan struct{} {
			if query.SkipArticle || query.StartDate.Year() == 2025 {
				return release
			}
			return nil
		},
	}
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{byAccount: map[string]accountClient{"synthetic": client}})
	profile, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{DisplayName: "Synthetic", Account: "synthetic", Password: "test-only", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	initial, err := app.RunSalesAnalysis(SalesAnalysisRequest{ProfileID: profile.ID, StoreIDs: []string{"107"}, Concurrency: 2, Periods: []SalesAnalysisPeriodRequest{
		{Key: "current", Label: "Current", From: "2026-08-01", To: "2026-08-31", IncludeTrend: true},
		{Key: "yearAgo", Label: "Year ago", From: "2025-08-01", To: "2025-08-31"},
	}})
	if err != nil {
		close(release)
		t.Fatal(err)
	}
	before, err := json.Marshal(initial)
	if err != nil {
		close(release)
		t.Fatal(err)
	}
	close(release)
	// Marshal the already-returned value while completion publishes its own value.
	for range 200 {
		if _, err := json.Marshal(initial); err != nil {
			t.Fatal(err)
		}
	}
	settled := waitSalesAnalysisSettled(t, app, initial.OperationID)
	after, err := json.Marshal(initial)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("published snapshot changed:\nbefore=%s\nafter=%s", before, after)
	}
	if !initial.Pending || len(initial.Periods) != 1 || initial.Periods[0].Totals.TransactionCount != nil {
		t.Fatal("initial snapshot was not article-only")
	}
	if settled.Pending || len(settled.Periods) != 2 || settled.Periods[0].Totals.TransactionCount == nil {
		t.Fatal("supplement was not published")
	}
}
