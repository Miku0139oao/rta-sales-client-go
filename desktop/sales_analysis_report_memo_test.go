package desktop

import "testing"

func TestGetSalesAnalysisReportMemoKeepsOneStoreAndDropsZeroGifts(t *testing.T) {
	period := SalesAnalysisPeriodResult{
		Key: "current",
		Stores: []SalesAnalysisStoreSummary{
			{BusinessID: "107", Label: "107 - Central"},
			{BusinessID: "108", Label: "108 - Harbour"},
		},
		Items: []SalesAnalysisItem{
			{StoreID: "107", ArticleCode: "A1", ArticleName: "Mask", Category2: "BEAUTY CARE", Category2Code: "A02", NetSalesAmount: 100, NetQuantity: 2},
			{StoreID: "108", ArticleCode: "B1", ArticleName: "Wipes", Category2: "HOUSEHOLD", Category2Code: "B12", NetSalesAmount: 80, NetQuantity: 4},
			{StoreID: "107", ArticleCode: "G1", ArticleName: "洗髮露贈品", Category2: "PC-FREE GIFT", Category2Code: "A07", NetSalesAmount: 0, NetQuantity: 8},
		},
	}
	app := &App{}
	app.rememberSalesAnalysis(SalesAnalysisResult{
		OperationID: "op-memo",
		Stores:      period.Stores,
		Periods:     []SalesAnalysisPeriodResult{period},
	}, map[string]SalesAnalysisPackedItems{"current": packSalesAnalysisItems(period)})

	all, err := app.GetSalesAnalysisReportMemo(SalesAnalysisReportMemoRequest{
		OperationID: "op-memo", CategoryLevel: "category2", ExcludeZeroGifts: true, Mode: "blacklist",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Periods) != 1 || len(all.Periods[0].TopAmount) != 2 {
		t.Fatalf("combined memo=%#v", all)
	}
	if all.Periods[0].TopAmount[0].Code != "A1" || all.Periods[0].TopAmount[1].Code != "B1" {
		t.Fatalf("top=%#v", all.Periods[0].TopAmount)
	}

	one, err := app.GetSalesAnalysisReportMemo(SalesAnalysisReportMemoRequest{
		OperationID: "op-memo", StoreID: "108", CategoryLevel: "category2", ExcludeZeroGifts: true, Mode: "blacklist",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(one.Periods[0].TopAmount) != 1 || one.Periods[0].TopAmount[0].Code != "B1" {
		t.Fatalf("store memo=%#v", one)
	}
}
