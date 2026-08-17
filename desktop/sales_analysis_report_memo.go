package desktop

import (
	"errors"
	"math"
	"regexp"
	"sort"
	"strings"
)

const (
	reportNominalAmount = 0.01
	reportTopProducts   = 15
	reportGroupItems    = 24
	reportFocusItems    = 8
)

var (
	giftCategoryPattern = regexp.MustCompile(`(?:^|[\s_-])(?:FREE[\s_-]*)?GIFT(?:[\s_-]|$)`)
	cashCouponPattern   = regexp.MustCompile(`現金(?:優惠)?券`)
	stampItemPattern    = regexp.MustCompile(`印花`)
	focusGroupPrefixes  = []struct {
		id     string
		prefix string
	}{
		{id: "health", prefix: "A01"},
		{id: "skin", prefix: "A02"},
		{id: "pc", prefix: "A03"},
	}
)

// GetSalesAnalysisReportMemo returns ranking and category totals for one PDF
// without sending article rows across the WebView bridge.
func (a *App) GetSalesAnalysisReportMemo(request SalesAnalysisReportMemoRequest) (SalesAnalysisReportMemo, error) {
	operationID := strings.TrimSpace(request.OperationID)
	if operationID == "" {
		return SalesAnalysisReportMemo{}, errors.New("operationId is required")
	}
	catalog := a.listedManCodeGroups()
	a.salesResultMu.Lock()
	defer a.salesResultMu.Unlock()
	if a.salesResult == nil || a.salesResult.OperationID != operationID {
		return SalesAnalysisReportMemo{}, errors.New("sales analysis result is no longer available")
	}
	level := strings.TrimSpace(request.CategoryLevel)
	if level == "" {
		level = "category2"
	}
	filter := reportMemoFilter{
		excludeZeroGifts: request.ExcludeZeroGifts,
		excludeStamps:    request.ExcludeStamps,
		mode:             strings.TrimSpace(request.Mode),
		categories:       request.Categories,
		facets:           request.Facets,
		search:           strings.TrimSpace(request.Search),
		uncategorized:    strings.TrimSpace(request.Uncategorized),
	}
	focusCatalog := catalog
	groupID := strings.TrimSpace(request.GroupID)
	if groupID != "" {
		filter.itemCodes = make(map[string]struct{})
		found := false
		for _, group := range catalog {
			if strings.TrimSpace(group.ID) != groupID {
				continue
			}
			found = true
			focusCatalog = []ManCodeGroup{group}
			for _, code := range group.Codes {
				code = strings.TrimSpace(code)
				if code != "" {
					filter.itemCodes[code] = struct{}{}
				}
			}
			break
		}
		if !found {
			return SalesAnalysisReportMemo{}, errors.New("mancode group does not exist")
		}
	}
	if filter.mode == "" {
		filter.mode = "blacklist"
	}
	storeID := strings.TrimSpace(request.StoreID)
	periods := make([]SalesAnalysisPeriodMemo, 0, len(a.salesResult.Periods))
	built := make(map[string]periodMemoBuilder, len(a.salesResult.Periods))
	for _, period := range a.salesResult.Periods {
		packed, ok := a.salesPacked[period.Key]
		if !ok {
			continue
		}
		builder := buildPeriodMemo(packed, periodStores(a.salesResult, period.Key), storeID, filter, level)
		built[period.Key] = builder
		memo := builder.finish(period.Key, "")
		if filter.itemCodes != nil {
			totals := builder.totals
			memo.Totals = &totals
		}
		periods = append(periods, memo)
	}
	if next, ok := built["yearAgoNext"]; ok {
		for index, period := range periods {
			if period.Key != "yearAgoNext" {
				continue
			}
			periods[index].FocusGroups = focusGroupsFromBuilders(next, built["current"], focusCatalog)
			periods[index].FocusCatalog = catalogFocusActive(focusCatalog)
		}
	}
	return SalesAnalysisReportMemo{Periods: periods}, nil
}

type reportMemoFilter struct {
	excludeZeroGifts bool
	excludeStamps    bool
	mode             string
	categories       []string
	facets           map[string][]string
	search           string
	uncategorized    string
	itemCodes        map[string]struct{}
}

type periodMemoBuilder struct {
	products   map[string]*SalesAnalysisRankedItem
	categories map[string]*memoCategory
	totals     SalesAnalysisTotals
}

type memoCategory struct {
	id       string
	code     string
	name     string
	amount   float64
	quantity float64
	products map[string]*SalesAnalysisRankedItem
}

func buildPeriodMemo(
	packed SalesAnalysisPackedItems,
	stores []SalesAnalysisStoreSummary,
	storeID string,
	filter reportMemoFilter,
	level string,
) periodMemoBuilder {
	storeIndex := -1
	if storeID != "" {
		for index, store := range stores {
			if store.BusinessID == storeID {
				storeIndex = index
				break
			}
		}
	}
	builder := periodMemoBuilder{
		products:   make(map[string]*SalesAnalysisRankedItem),
		categories: make(map[string]*memoCategory),
	}
	if storeID != "" && storeIndex < 0 {
		return builder
	}
	for _, row := range packed.Rows {
		if storeIndex >= 0 && row.S != storeIndex {
			continue
		}
		if !includePackedReportRow(packed.Dict, row, stores, filter, level) {
			continue
		}
		builder.totals.SaleQuantity += row.Sq
		builder.totals.SaleAmount += row.Sa
		builder.totals.ReturnQuantity += row.Rq
		builder.totals.ReturnAmount += row.Ra
		builder.totals.NetQuantity += row.Nq
		builder.totals.NetSalesAmount += row.Ns
		code := packedString(packed.Dict, row.Ac)
		name := packedString(packed.Dict, row.An)
		id := strings.TrimSpace(code)
		if id == "" {
			id = strings.TrimSpace(name)
		}
		if id == "" {
			continue
		}
		product := builder.products[id]
		if product == nil {
			product = &SalesAnalysisRankedItem{
				ID:            id,
				Code:          strings.TrimSpace(code),
				Name:          strings.TrimSpace(name),
				Brand:         strings.TrimSpace(packedString(packed.Dict, row.Br)),
				Category2Code: strings.TrimSpace(packedString(packed.Dict, row.K2)),
				Category3Code: strings.TrimSpace(packedString(packed.Dict, row.K3)),
				Category4Code: strings.TrimSpace(packedString(packed.Dict, row.K4)),
			}
			builder.products[id] = product
		}
		product.Amount += row.Ns
		product.Quantity += row.Nq
		if product.Name == "" && strings.TrimSpace(name) != "" {
			product.Name = strings.TrimSpace(name)
		}
		categoryCode, categoryName := packedCategory(packed.Dict, row, level)
		categoryID := categoryCode
		if categoryID == "" {
			categoryID = categoryName
		}
		if categoryID == "" {
			continue
		}
		category := builder.categories[categoryID]
		if category == nil {
			category = &memoCategory{
				id: categoryID, code: categoryCode, name: categoryName,
				products: make(map[string]*SalesAnalysisRankedItem),
			}
			builder.categories[categoryID] = category
		}
		category.amount += row.Ns
		category.quantity += row.Nq
		if category.name == "" && categoryName != "" {
			category.name = categoryName
		}
		ranked := category.products[id]
		if ranked == nil {
			ranked = &SalesAnalysisRankedItem{ID: id, Code: product.Code, Name: product.Name, Brand: product.Brand}
			category.products[id] = ranked
		}
		ranked.Amount += row.Ns
		ranked.Quantity += row.Nq
		if ranked.Name == "" && product.Name != "" {
			ranked.Name = product.Name
		}
	}
	return builder
}

func (b periodMemoBuilder) finish(key, uncategorized string) SalesAnalysisPeriodMemo {
	if uncategorized == "" {
		uncategorized = "未分類"
	}
	products := rankedValues(b.products)
	groups := make([]SalesAnalysisCategoryGroup, 0, len(b.categories))
	for _, category := range b.categories {
		name := category.name
		if name == "" {
			name = uncategorized
		}
		items := rankedValues(category.products)
		if len(items) == 0 {
			continue
		}
		groups = append(groups, SalesAnalysisCategoryGroup{
			ID: category.id, Code: category.code, Name: name,
			Amount: category.amount, Quantity: category.quantity,
			Items: items,
		})
	}
	amountGroups := append([]SalesAnalysisCategoryGroup(nil), groups...)
	quantityGroups := append([]SalesAnalysisCategoryGroup(nil), groups...)
	sort.Slice(amountGroups, func(i, j int) bool {
		return rankedGreater(amountGroups[i].Amount, amountGroups[j].Amount, amountGroups[i].ID, amountGroups[j].ID)
	})
	sort.Slice(quantityGroups, func(i, j int) bool {
		return rankedGreater(quantityGroups[i].Quantity, quantityGroups[j].Quantity, quantityGroups[i].ID, quantityGroups[j].ID)
	})
	for index := range amountGroups {
		amountGroups[index].Items = topRanked(amountGroups[index].Items, "amount", reportGroupItems)
	}
	for index := range quantityGroups {
		quantityGroups[index].Items = topRanked(quantityGroups[index].Items, "quantity", reportGroupItems)
	}
	return SalesAnalysisPeriodMemo{
		Key:            key,
		TopAmount:      topRanked(products, "amount", reportTopProducts),
		TopQuantity:    topRanked(products, "quantity", reportTopProducts),
		AmountGroups:   amountGroups,
		QuantityGroups: quantityGroups,
	}
}

func rankedValues(source map[string]*SalesAnalysisRankedItem) []SalesAnalysisRankedItem {
	items := make([]SalesAnalysisRankedItem, 0, len(source))
	for _, item := range source {
		items = append(items, *item)
	}
	return items
}

func topRanked(items []SalesAnalysisRankedItem, metric string, limit int) []SalesAnalysisRankedItem {
	sorted := append([]SalesAnalysisRankedItem(nil), items...)
	sort.Slice(sorted, func(i, j int) bool {
		if metric == "quantity" {
			return rankedGreater(sorted[i].Quantity, sorted[j].Quantity, sorted[i].ID, sorted[j].ID)
		}
		return rankedGreater(sorted[i].Amount, sorted[j].Amount, sorted[i].ID, sorted[j].ID)
	})
	if len(sorted) > limit {
		return sorted[:limit]
	}
	return sorted
}

func rankedGreater(left, right float64, leftID, rightID string) bool {
	if left != right {
		return left > right
	}
	return leftID < rightID
}

func includePackedReportRow(dict []string, row SalesAnalysisPackedRow, stores []SalesAnalysisStoreSummary, filter reportMemoFilter, level string) bool {
	if filter.itemCodes != nil {
		code := strings.TrimSpace(packedString(dict, row.Ac))
		if _, ok := filter.itemCodes[code]; !ok {
			return false
		}
	}
	text := packedString(dict, row.An) + " " + packedString(dict, row.Ac) + " " + packedString(dict, row.Br)
	categoryNames := []string{
		packedString(dict, row.C1), packedString(dict, row.C2), packedString(dict, row.C3),
		packedString(dict, row.C4), packedString(dict, row.C5),
	}
	categoryFields := append(append([]string{}, categoryNames...),
		packedString(dict, row.K1), packedString(dict, row.K2), packedString(dict, row.K3),
		packedString(dict, row.K4), packedString(dict, row.K5),
	)
	if filter.excludeStamps && (stampItemPattern.MatchString(text) || anyMatches(categoryNames, stampItemPattern)) {
		return false
	}
	if filter.excludeZeroGifts && math.Abs(row.Ns) <= reportNominalAmount && isGiftCategoryText(categoryFields) && !cashCouponPattern.MatchString(text) {
		return false
	}
	if !packedRowMatchesFacets(dict, row, filter) {
		return false
	}
	if !packedRowMatchesSearch(dict, row, stores, filter.search) {
		return false
	}
	if len(filter.categories) == 0 {
		return filter.mode != "whitelist"
	}
	code, name := packedCategory(dict, row, level)
	id := code
	if id == "" {
		id = name
	}
	listed := containsString(filter.categories, id)
	if filter.mode == "whitelist" {
		return listed
	}
	return !listed
}

func packedRowMatchesFacets(dict []string, row SalesAnalysisPackedRow, filter reportMemoFilter) bool {
	if len(filter.facets) == 0 {
		return true
	}
	for _, level := range []string{"category1", "category2", "category3", "category4", "category5"} {
		selected := filter.facets[level]
		if len(selected) == 0 {
			continue
		}
		code, name := packedCategory(dict, row, level)
		if !categorySelectionContains(selected, code, name, filter.uncategorized) {
			return false
		}
	}
	return true
}

func packedRowMatchesSearch(dict []string, row SalesAnalysisPackedRow, stores []SalesAnalysisStoreSummary, search string) bool {
	term := strings.ToLower(strings.TrimSpace(search))
	if term == "" {
		return true
	}
	storeID, storeLabel := "", ""
	if row.S >= 0 && row.S < len(stores) {
		storeID = stores[row.S].BusinessID
		storeLabel = stores[row.S].Label
	}
	fields := []string{
		storeID, storeLabel,
		packedString(dict, row.Ac), packedString(dict, row.An), packedString(dict, row.Br),
		packedString(dict, row.C1), packedString(dict, row.K1),
		packedString(dict, row.C2), packedString(dict, row.K2),
		packedString(dict, row.C3), packedString(dict, row.K3),
		packedString(dict, row.C4), packedString(dict, row.K4),
		packedString(dict, row.C5), packedString(dict, row.K5),
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), term) {
			return true
		}
	}
	return false
}

func categorySelectionContains(selected []string, code, name, uncategorized string) bool {
	label := reportCategoryLabel(code, name, uncategorized)
	fallbackName := name
	if fallbackName == "" {
		fallbackName = reportUncategorized(uncategorized)
	}
	for _, value := range selected {
		if value == label || value == fallbackName || (code != "" && value == code) {
			return true
		}
	}
	return false
}

func reportCategoryLabel(code, name, uncategorized string) string {
	if code != "" && name != "" && name != code {
		return code + "  " + name
	}
	if name != "" {
		return name
	}
	if code != "" {
		return code
	}
	return reportUncategorized(uncategorized)
}

func reportUncategorized(value string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return "未分類"
}

func isGiftCategoryText(values []string) bool {
	for _, value := range values {
		text := strings.TrimSpace(value)
		if text == "" {
			continue
		}
		if giftCategoryPattern.MatchString(strings.ToUpper(text)) || strings.Contains(text, "贈品") {
			return true
		}
	}
	return false
}

func anyMatches(values []string, pattern *regexp.Regexp) bool {
	for _, value := range values {
		if pattern.MatchString(value) {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func packedCategory(dict []string, row SalesAnalysisPackedRow, level string) (code, name string) {
	switch level {
	case "category1":
		return strings.TrimSpace(packedString(dict, row.K1)), strings.TrimSpace(packedString(dict, row.C1))
	case "category3":
		return strings.TrimSpace(packedString(dict, row.K3)), strings.TrimSpace(packedString(dict, row.C3))
	case "category4":
		return strings.TrimSpace(packedString(dict, row.K4)), strings.TrimSpace(packedString(dict, row.C4))
	case "category5":
		return strings.TrimSpace(packedString(dict, row.K5)), strings.TrimSpace(packedString(dict, row.C5))
	default:
		return strings.TrimSpace(packedString(dict, row.K2)), strings.TrimSpace(packedString(dict, row.C2))
	}
}

type focusGroupSpec struct {
	id     string
	name   string
	prefix string
	codes  map[string]struct{}
}

func catalogFocusActive(catalog []ManCodeGroup) bool {
	for _, group := range catalog {
		for _, code := range group.Codes {
			if strings.TrimSpace(code) != "" {
				return true
			}
		}
	}
	return false
}

func focusGroupSpecs(catalog []ManCodeGroup) []focusGroupSpec {
	specs := make([]focusGroupSpec, 0, len(catalog))
	for _, group := range catalog {
		codes := make(map[string]struct{}, len(group.Codes))
		for _, code := range group.Codes {
			code = strings.TrimSpace(code)
			if code == "" {
				continue
			}
			codes[code] = struct{}{}
		}
		if len(codes) == 0 {
			continue
		}
		specs = append(specs, focusGroupSpec{id: group.ID, name: group.Name, codes: codes})
	}
	if len(specs) == 0 {
		for _, spec := range focusGroupPrefixes {
			specs = append(specs, focusGroupSpec{id: spec.id, prefix: spec.prefix})
		}
	}
	return specs
}

func focusGroupsFromBuilders(yearAgoNext, current periodMemoBuilder, catalog []ManCodeGroup) []SalesAnalysisFocusGroup {
	specs := focusGroupSpecs(catalog)
	groups := make([]SalesAnalysisFocusGroup, 0, len(specs))
	for _, spec := range specs {
		ranked := make([]SalesAnalysisFocusProduct, 0)
		for _, product := range yearAgoNext.products {
			if !productMatchesFocusSpec(*product, spec) {
				continue
			}
			live := current.products[product.ID]
			if live == nil {
				live = current.products[product.Code]
			}
			item := SalesAnalysisFocusProduct{
				ID: product.ID, Code: product.Code, Name: product.Name, Brand: product.Brand,
				Amount: product.Amount, Quantity: product.Quantity,
			}
			if live != nil {
				item.CurrentAmount = live.Amount
				item.CurrentQuantity = live.Quantity
			}
			ranked = append(ranked, item)
		}
		sales := append([]SalesAnalysisFocusProduct(nil), ranked...)
		quantity := append([]SalesAnalysisFocusProduct(nil), ranked...)
		sort.Slice(sales, func(i, j int) bool { return rankedGreater(sales[i].Amount, sales[j].Amount, sales[i].ID, sales[j].ID) })
		sort.Slice(quantity, func(i, j int) bool {
			return rankedGreater(quantity[i].Quantity, quantity[j].Quantity, quantity[i].ID, quantity[j].ID)
		})
		if len(sales) > reportFocusItems {
			sales = sales[:reportFocusItems]
		}
		if len(quantity) > reportFocusItems {
			quantity = quantity[:reportFocusItems]
		}
		if len(sales) == 0 && len(quantity) == 0 {
			continue
		}
		groups = append(groups, SalesAnalysisFocusGroup{
			ID: spec.id, Prefix: spec.prefix, Name: spec.name, Sales: sales, Quantity: quantity,
		})
	}
	return groups
}

func productMatchesFocusSpec(product SalesAnalysisRankedItem, spec focusGroupSpec) bool {
	if len(spec.codes) > 0 {
		code := strings.TrimSpace(product.Code)
		if code == "" {
			code = strings.TrimSpace(product.ID)
		}
		_, ok := spec.codes[code]
		return ok
	}
	return productMatchesFocus(product, spec.prefix)
}

func productMatchesFocus(product SalesAnalysisRankedItem, prefix string) bool {
	department := strings.TrimSpace(product.Category2Code)
	if department != "" {
		return department == prefix || strings.HasPrefix(department, prefix)
	}
	fallback := strings.TrimSpace(product.Category3Code)
	if fallback == "" {
		fallback = strings.TrimSpace(product.Category4Code)
	}
	return fallback == prefix || strings.HasPrefix(fallback, prefix)
}
