package desktop

import (
	"encoding/json"
	"strings"
	"testing"

	rtasales "github.com/Miku0139oao/rta-sales-client-go/rtasales"
)

func TestPackSalesAnalysisItemsInternsRepeatedText(t *testing.T) {
	period := SalesAnalysisPeriodResult{
		Key: "current",
		Stores: []SalesAnalysisStoreSummary{
			{BusinessID: "107", Label: "107 - Central"},
			{BusinessID: "108", Label: "108 - Harbour"},
		},
		Items: []SalesAnalysisItem{
			{
				StoreID: "107", StoreLabel: "107 - Central", ArticleCode: "A1", ArticleName: "Mask",
				Category1: "HEALTH", Category1Code: "A", NetSalesAmount: 10, SaleAmount: 10,
			},
			{
				StoreID: "108", StoreLabel: "108 - Harbour", ArticleCode: "A1", ArticleName: "Mask",
				Category1: "HEALTH", Category1Code: "A", NetSalesAmount: 8, SaleAmount: 8,
			},
		},
	}
	packed := packSalesAnalysisItems(period)
	if len(packed.Rows) != 2 {
		t.Fatalf("rows=%d, want 2", len(packed.Rows))
	}
	if packed.Rows[0].Ac != packed.Rows[1].Ac || packed.Rows[0].C1 != packed.Rows[1].C1 {
		t.Fatalf("repeated article and category text was not interned: %+v", packed)
	}
	unpacked := unpackSalesAnalysisItems(packed, period.Stores)
	if unpacked[0].StoreID != "107" || unpacked[1].StoreLabel != "108 - Harbour" || unpacked[0].ArticleName != "Mask" {
		t.Fatalf("unpacked=%#v", unpacked)
	}
	raw, err := json.Marshal(packed)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"ac":`) || strings.Contains(string(raw), `"periodKey"`) {
		t.Fatalf("packed JSON still uses object rows: %s", raw)
	}
	var roundtrip SalesAnalysisPackedItems
	if err := json.Unmarshal(raw, &roundtrip); err != nil {
		t.Fatal(err)
	}
	again := unpackSalesAnalysisItems(roundtrip, period.Stores)
	if again[0].ArticleName != "Mask" || again[1].NetSalesAmount != 8 {
		t.Fatalf("roundtrip=%#v", again)
	}
}

func TestAttachPeriodSummaryOmitsItemRowsFromTheSlimPeriod(t *testing.T) {
	period := SalesAnalysisPeriodResult{
		Key: "current",
		Stores: []SalesAnalysisStoreSummary{
			{BusinessID: "107", Label: "107 - Central"},
		},
		Items: []SalesAnalysisItem{
			{StoreID: "107", ArticleCode: "A1", ArticleName: "Mask", Category2: "BEAUTY CARE", Category2Code: "A02", NetSalesAmount: 100, NetQuantity: 2},
			{StoreID: "107", ArticleCode: "B1", ArticleName: "Wipes", Category2: "HOUSEHOLD", Category2Code: "B12", NetSalesAmount: 80, NetQuantity: 4},
		},
	}
	packed := packSalesAnalysisItems(period)
	period.Items = nil
	attachPeriodSummary(&period, packed)
	if period.ItemCount != 2 {
		t.Fatalf("itemCount=%d", period.ItemCount)
	}
	if len(period.Items) != 0 {
		t.Fatalf("summary should not keep article rows: %#v", period.Items)
	}
	if len(period.TopAmount) != 2 || period.TopAmount[0].Code != "A1" {
		t.Fatalf("topAmount=%#v", period.TopAmount)
	}
	if len(period.FacetOptions["category2"]) != 2 {
		t.Fatalf("facetOptions=%#v", period.FacetOptions)
	}
	if len(period.CategoryGroups["category2"]) != 2 {
		t.Fatalf("categoryGroups=%#v", period.CategoryGroups)
	}
}

func TestClearSalesAnalysisDropsPackedRowsAfterResultIsDiscarded(t *testing.T) {
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{byAccount: map[string]accountClient{
		"analysis-account": &salesAnalysisFakeClient{
			stores:  []rtasales.Store{{BusinessID: "107", Label: "107"}},
			results: map[string]*rtasales.SalesResult{"107": {Items: []rtasales.SaleItem{{Matnr: "A1", TPGrossSaleAmount: 10}}}},
		},
	}})
	profile, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		DisplayName: "Analysis", Account: "analysis-account", Password: "password", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := app.RunSalesAnalysis(SalesAnalysisRequest{
		ProfileID: profile.ID, StoreIDs: []string{"107"}, From: "2026-08-15", To: "2026-08-15",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.GetSalesAnalysisItems(SalesAnalysisItemsRequest{OperationID: result.OperationID, PeriodKey: "current"}); err != nil {
		t.Fatal(err)
	}
	if err := app.ClearSalesAnalysis(OperationRequest{OperationID: result.OperationID}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.GetSalesAnalysisItems(SalesAnalysisItemsRequest{OperationID: result.OperationID, PeriodKey: "current"}); err == nil {
		t.Fatal("expected packed rows to be released after clear")
	}
}

func TestGetSalesAnalysisItemsCanReturnOneStore(t *testing.T) {
	period := SalesAnalysisPeriodResult{
		Key: "current",
		Stores: []SalesAnalysisStoreSummary{
			{BusinessID: "107", Label: "107 - Central"},
			{BusinessID: "108", Label: "108 - Harbour"},
		},
		Items: []SalesAnalysisItem{
			{StoreID: "107", StoreLabel: "107 - Central", ArticleCode: "A1", ArticleName: "Mask", NetSalesAmount: 10},
			{StoreID: "108", StoreLabel: "108 - Harbour", ArticleCode: "B1", ArticleName: "Wipes", NetSalesAmount: 8},
		},
	}
	app := &App{}
	app.rememberSalesAnalysis(SalesAnalysisResult{
		OperationID: "op-store",
		Stores:      period.Stores,
		Periods:     []SalesAnalysisPeriodResult{period},
	}, map[string]SalesAnalysisPackedItems{"current": packSalesAnalysisItems(period)})
	packed, err := app.GetSalesAnalysisItems(SalesAnalysisItemsRequest{
		OperationID: "op-store", PeriodKey: "current", StoreID: "108",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(packed.Rows) != 1 {
		t.Fatalf("rows=%d, want 1", len(packed.Rows))
	}
	items := unpackSalesAnalysisItems(packed, []SalesAnalysisStoreSummary{{BusinessID: "108", Label: "108 - Harbour"}})
	if items[0].ArticleCode != "B1" || items[0].StoreID != "108" {
		t.Fatalf("unpacked=%#v", items[0])
	}
	glyphs, err := app.GetSalesAnalysisReportGlyphs(OperationRequest{OperationID: "op-store"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"M", "k", "W", "1", "0", "7", "8"} {
		if !strings.Contains(glyphs, want) {
			t.Fatalf("glyphs %q missing %q", glyphs, want)
		}
	}
}

func TestSlimSalesAnalysisKeepsPackedItemCount(t *testing.T) {
	slim := slimSalesAnalysis(SalesAnalysisResult{
		Periods: []SalesAnalysisPeriodResult{{
			Key: "current", ItemCount: 16, Items: nil,
		}},
	})
	if slim.Periods[0].ItemCount != 16 {
		t.Fatalf("itemCount=%d, want 16", slim.Periods[0].ItemCount)
	}
}

func TestSalesAnalysisRowsRoundsCountsToWholeUnits(t *testing.T) {
	items, totals, err := salesAnalysisRows(rtasales.Store{BusinessID: "107", Label: "107"}, &rtasales.SalesResult{
		Items: []rtasales.SaleItem{{
			Matnr: "A1", TPSaleQuantity: 1.4, TPReturnSaleQuantity: 0.6, TPGrossSaleQuantity: 2.5,
			TPTransactionCount: 3.2, TPReturnTransactionCount: 0.8, TPSaleAmount: 10.25, TPGrossSaleAmount: 9.5,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if items[0].SaleQuantity != 1 || items[0].ReturnQuantity != 1 || items[0].NetQuantity != 3 ||
		items[0].TransactionCount != 3 || items[0].ReturnTransactionCount != 1 {
		t.Fatalf("counts were not whole units: %#v", items[0])
	}
	if totals.SaleQuantity != 1 || totals.ReturnQuantity != 1 || totals.NetQuantity != 3 {
		t.Fatalf("totals were not whole units: %#v", totals)
	}
}
