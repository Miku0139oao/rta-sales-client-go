package desktop

import (
	"slices"
	"testing"
)

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
	if all.Periods[0].Totals == nil || all.Periods[0].Totals.NetSalesAmount != 180 || all.Periods[0].Totals.NetQuantity != 6 {
		t.Fatalf("unscoped memo totals=%#v, want item-sum 180 / 6", all.Periods[0].Totals)
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

func TestGetSalesAnalysisReportMemoAppliesAnalysisFacets(t *testing.T) {
	period := SalesAnalysisPeriodResult{
		Key: "current",
		Stores: []SalesAnalysisStoreSummary{
			{BusinessID: "107", Label: "107 - Central"},
		},
		Items: []SalesAnalysisItem{
			{StoreID: "107", ArticleCode: "A1", ArticleName: "Spray", Category5: "RESPIRATORY SYSTEM", Category5Code: "A01010203", NetSalesAmount: 100, NetQuantity: 2},
			{StoreID: "107", ArticleCode: "B1", ArticleName: "Vitamin", Category5: "NUTRITION", Category5Code: "A01010207", NetSalesAmount: 80, NetQuantity: 4},
		},
	}
	app := &App{}
	app.rememberSalesAnalysis(SalesAnalysisResult{
		OperationID: "op-facets",
		Stores:      period.Stores,
		Periods:     []SalesAnalysisPeriodResult{period},
	}, map[string]SalesAnalysisPackedItems{"current": packSalesAnalysisItems(period)})

	memo, err := app.GetSalesAnalysisReportMemo(SalesAnalysisReportMemoRequest{
		OperationID: "op-facets", CategoryLevel: "category2", Mode: "blacklist",
		Facets: map[string][]string{"category5": {"A01010203  RESPIRATORY SYSTEM"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(memo.Periods) != 1 || len(memo.Periods[0].TopAmount) != 1 || memo.Periods[0].TopAmount[0].Code != "A1" {
		t.Fatalf("facet memo=%#v", memo)
	}
}

func TestGetSalesAnalysisReportMemoScopesProductsByManCodeGroup(t *testing.T) {
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{})
	groupA, err := app.SaveManCodeGroup(SaveManCodeGroupRequest{
		Name: "Promoter A", Codes: []string{" H1 ", "H2", "S1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	groupB, err := app.SaveManCodeGroup(SaveManCodeGroupRequest{
		Name: "Promoter B", Codes: []string{"B1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := app.SaveManCodeGroup(SaveManCodeGroupRequest{Name: "Empty promoter"})
	if err != nil {
		t.Fatal(err)
	}

	period := SalesAnalysisPeriodResult{
		Key: "current",
		Stores: []SalesAnalysisStoreSummary{
			{BusinessID: "107", Label: "107 - Central"},
			{BusinessID: "108", Label: "108 - Harbour"},
		},
		Items: []SalesAnalysisItem{
			{StoreID: "107", ArticleCode: " H1 ", ArticleName: "Health One", Category2: "HEALTH", Category2Code: "A01", SaleAmount: 120, ReturnAmount: 20, NetSalesAmount: 100, SaleQuantity: 3, ReturnQuantity: 1, NetQuantity: 2},
			{StoreID: "107", ArticleCode: "H10", ArticleName: "Exact-match neighbour", Category2: "HEALTH", Category2Code: "A01", SaleAmount: 900, NetSalesAmount: 900, SaleQuantity: 11, NetQuantity: 11},
			{StoreID: "107", ArticleCode: "S1", ArticleName: "Skin One", Category2: "SKIN", Category2Code: "A02", SaleAmount: 220, ReturnAmount: 20, NetSalesAmount: 200, SaleQuantity: 5, ReturnQuantity: 1, NetQuantity: 4},
			{StoreID: "108", ArticleCode: "H2", ArticleName: "Health Two", Category2: "HEALTH", Category2Code: "A01", SaleAmount: 330, ReturnAmount: 30, NetSalesAmount: 300, SaleQuantity: 7, ReturnQuantity: 1, NetQuantity: 6},
			{StoreID: "108", ArticleCode: "B1", ArticleName: "Brand One", Category2: "HOUSEHOLD", Category2Code: "B12", SaleAmount: 440, ReturnAmount: 40, NetSalesAmount: 400, SaleQuantity: 9, ReturnQuantity: 1, NetQuantity: 8},
		},
	}
	app.rememberSalesAnalysis(SalesAnalysisResult{
		OperationID: "op-groups",
		Stores:      period.Stores,
		Periods:     []SalesAnalysisPeriodResult{period},
	}, map[string]SalesAnalysisPackedItems{"current": packSalesAnalysisItems(period)})

	tests := []struct {
		name       string
		req        SalesAnalysisReportMemoRequest
		want       []string
		wantTotals SalesAnalysisTotals
	}{
		{
			name: "group A uses an exact trimmed article-code match",
			req:  SalesAnalysisReportMemoRequest{OperationID: "op-groups", GroupID: groupA.ID},
			want: []string{"H2", "S1", "H1"},
			wantTotals: SalesAnalysisTotals{
				SaleQuantity: 15, SaleAmount: 670, ReturnQuantity: 3, ReturnAmount: 70,
				NetQuantity: 12, NetSalesAmount: 600,
			},
		},
		{
			name: "group B has an independent report scope",
			req:  SalesAnalysisReportMemoRequest{OperationID: "op-groups", GroupID: groupB.ID},
			want: []string{"B1"},
			wantTotals: SalesAnalysisTotals{
				SaleQuantity: 9, SaleAmount: 440, ReturnQuantity: 1, ReturnAmount: 40,
				NetQuantity: 8, NetSalesAmount: 400,
			},
		},
		{
			name: "group and store are intersected",
			req:  SalesAnalysisReportMemoRequest{OperationID: "op-groups", GroupID: groupA.ID, StoreID: "107"},
			want: []string{"S1", "H1"},
			wantTotals: SalesAnalysisTotals{
				SaleQuantity: 8, SaleAmount: 340, ReturnQuantity: 2, ReturnAmount: 40,
				NetQuantity: 6, NetSalesAmount: 300,
			},
		},
		{
			name: "group and category whitelist are intersected",
			req: SalesAnalysisReportMemoRequest{
				OperationID: "op-groups", GroupID: groupA.ID, CategoryLevel: "category2",
				Mode: "whitelist", Categories: []string{"A02"},
			},
			want: []string{"S1"},
			wantTotals: SalesAnalysisTotals{
				SaleQuantity: 5, SaleAmount: 220, ReturnQuantity: 1, ReturnAmount: 20,
				NetQuantity: 4, NetSalesAmount: 200,
			},
		},
		{
			name:       "empty group produces an empty product memo",
			req:        SalesAnalysisReportMemoRequest{OperationID: "op-groups", GroupID: empty.ID},
			want:       []string{},
			wantTotals: SalesAnalysisTotals{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			memo, err := app.GetSalesAnalysisReportMemo(test.req)
			if err != nil {
				t.Fatal(err)
			}
			if len(memo.Periods) != 1 {
				t.Fatalf("periods=%#v", memo.Periods)
			}
			period := memo.Periods[0]
			got := make([]string, 0, len(period.TopAmount))
			for _, item := range period.TopAmount {
				got = append(got, item.Code)
			}
			if !slices.Equal(got, test.want) {
				t.Fatalf("top codes=%v, want %v", got, test.want)
			}
			if period.Totals == nil {
				t.Fatal("group-scoped memo omitted totals")
			}
			if *period.Totals != test.wantTotals {
				t.Fatalf("totals=%#v, want %#v", *period.Totals, test.wantTotals)
			}
			if period.Totals.TransactionCount != nil || period.Totals.TrendNetSalesAmount != nil {
				t.Fatalf("SKU rows must not synthesize transaction or trend totals: %#v", period.Totals)
			}
			if len(test.want) == 0 && (len(period.TopQuantity) != 0 || len(period.AmountGroups) != 0 || len(period.QuantityGroups) != 0) {
				t.Fatalf("empty group memo=%#v", period)
			}
		})
	}

	if _, err := app.GetSalesAnalysisReportMemo(SalesAnalysisReportMemoRequest{
		OperationID: "op-groups", GroupID: "missing-group",
	}); err == nil {
		t.Fatal("unknown groupId should fail")
	}
}

func TestGetSalesAnalysisReportMemoScopesFocusToSelectedManCodeGroup(t *testing.T) {
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{})
	selected, err := app.SaveManCodeGroup(SaveManCodeGroupRequest{Name: "Selected", Codes: []string{"H1"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.SaveManCodeGroup(SaveManCodeGroupRequest{Name: "Overlapping", Codes: []string{"H1"}}); err != nil {
		t.Fatal(err)
	}
	memo := reportMemoWithFocusCatalogRequest(t, app, SalesAnalysisReportMemoRequest{GroupID: selected.ID})
	period := yearAgoNextMemo(memo)
	if len(period.FocusGroups) != 1 || period.FocusGroups[0].ID != selected.ID {
		t.Fatalf("selected focus groups=%#v", period.FocusGroups)
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
	return reportMemoWithFocusCatalogRequest(t, app, SalesAnalysisReportMemoRequest{})
}

func reportMemoWithFocusCatalogRequest(t *testing.T, app *App, request SalesAnalysisReportMemoRequest) SalesAnalysisReportMemo {
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
		Key:    "current",
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
	request.OperationID = "op-focus"
	memo, err := app.GetSalesAnalysisReportMemo(request)
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
