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

func TestGetSalesAnalysisReportMemoFocusGroupsFallbackToPrefixes(t *testing.T) {
	memo := reportMemoWithFocusCatalog(t, &App{})
	period := yearAgoNextMemo(memo)
	if period.FocusCatalog {
		t.Fatalf("empty catalog should not set focusCatalog: %#v", period)
	}
	groups := period.FocusGroups
	if len(groups) != 2 || groups[0].ID != "health" || groups[0].Prefix != "A01" || groups[1].ID != "skin" {
		t.Fatalf("fallback groups=%#v", groups)
	}
	if len(groups[0].Sales) != 2 || groups[0].Sales[0].Code != "H1" || groups[0].Sales[1].Code != "H2" {
		t.Fatalf("health sales=%#v", groups[0].Sales)
	}
	if groups[0].Sales[0].CurrentAmount != 30 {
		t.Fatalf("current amount=%#v", groups[0].Sales[0])
	}
}

func TestGetSalesAnalysisReportMemoFocusGroupsUseCatalogCodes(t *testing.T) {
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{})
	created, err := app.SaveManCodeGroup(SaveManCodeGroupRequest{Name: "保健", Codes: []string{"H1"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.SaveManCodeGroup(SaveManCodeGroupRequest{Name: "空組"}); err != nil {
		t.Fatal(err)
	}
	memo := reportMemoWithFocusCatalog(t, app)
	period := yearAgoNextMemo(memo)
	if !period.FocusCatalog {
		t.Fatalf("catalog memo should set focusCatalog: %#v", period)
	}
	groups := period.FocusGroups
	if len(groups) != 1 || groups[0].ID != created.ID || groups[0].Name != "保健" {
		t.Fatalf("catalog groups=%#v", groups)
	}
	if len(groups[0].Sales) != 1 || groups[0].Sales[0].Code != "H1" {
		t.Fatalf("catalog sales=%#v", groups[0].Sales)
	}
}

func TestGetSalesAnalysisReportMemoFocusGroupsEmptyCatalogGroupsFallback(t *testing.T) {
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{})
	if _, err := app.SaveManCodeGroup(SaveManCodeGroupRequest{Name: "空組"}); err != nil {
		t.Fatal(err)
	}
	memo := reportMemoWithFocusCatalog(t, app)
	period := yearAgoNextMemo(memo)
	if period.FocusCatalog {
		t.Fatalf("empty-code catalog should not set focusCatalog: %#v", period)
	}
	groups := period.FocusGroups
	if len(groups) != 2 || groups[0].ID != "health" || groups[1].ID != "skin" {
		t.Fatalf("empty-code catalog should keep prefixes: %#v", groups)
	}
}

func TestGetSalesAnalysisReportMemoFocusGroupsCatalogMissStaysEmpty(t *testing.T) {
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{})
	if _, err := app.SaveManCodeGroup(SaveManCodeGroupRequest{Name: "未命中", Codes: []string{"NOPE"}}); err != nil {
		t.Fatal(err)
	}
	memo := reportMemoWithFocusCatalog(t, app)
	period := yearAgoNextMemo(memo)
	if !period.FocusCatalog {
		t.Fatalf("catalog miss should still mark focusCatalog: %#v", period)
	}
	if len(period.FocusGroups) != 0 {
		t.Fatalf("catalog miss should not resurrect prefixes: %#v", period.FocusGroups)
	}
}

func reportMemoWithFocusCatalog(t *testing.T, app *App) SalesAnalysisReportMemo {
	t.Helper()
	yearAgoNext := SalesAnalysisPeriodResult{
		Key: "yearAgoNext",
		Stores: []SalesAnalysisStoreSummary{
			{BusinessID: "107", Label: "107 - Central"},
		},
		Items: []SalesAnalysisItem{
			{StoreID: "107", ArticleCode: "H1", ArticleName: "Oil", Category2: "HEALTH CARE", Category2Code: "A01", NetSalesAmount: 200, NetQuantity: 4},
			{StoreID: "107", ArticleCode: "H2", ArticleName: "Pain", Category2: "HEALTH CARE", Category2Code: "A01", NetSalesAmount: 80, NetQuantity: 20},
			{StoreID: "107", ArticleCode: "S1", ArticleName: "Mask", Category2: "BEAUTY CARE", Category2Code: "A02", NetSalesAmount: 150, NetQuantity: 8},
			{StoreID: "107", ArticleCode: "X1", ArticleName: "Snack", Category2: "FOOD", Category2Code: "E02", NetSalesAmount: 999, NetQuantity: 99},
		},
	}
	current := SalesAnalysisPeriodResult{
		Key: "current",
		Stores: yearAgoNext.Stores,
		Items: []SalesAnalysisItem{
			{StoreID: "107", ArticleCode: "H1", ArticleName: "Oil", Category2: "HEALTH CARE", Category2Code: "A01", NetSalesAmount: 30, NetQuantity: 1},
		},
	}
	app.rememberSalesAnalysis(SalesAnalysisResult{
		OperationID: "op-focus",
		Stores:      yearAgoNext.Stores,
		Periods:     []SalesAnalysisPeriodResult{current, yearAgoNext},
	}, map[string]SalesAnalysisPackedItems{
		"current":     packSalesAnalysisItems(current),
		"yearAgoNext": packSalesAnalysisItems(yearAgoNext),
	})
	memo, err := app.GetSalesAnalysisReportMemo(SalesAnalysisReportMemoRequest{OperationID: "op-focus"})
	if err != nil {
		t.Fatal(err)
	}
	return memo
}

func yearAgoNextMemo(memo SalesAnalysisReportMemo) SalesAnalysisPeriodMemo {
	for _, period := range memo.Periods {
		if period.Key == "yearAgoNext" {
			return period
		}
	}
	return SalesAnalysisPeriodMemo{}
}
