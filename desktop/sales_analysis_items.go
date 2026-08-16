package desktop

import (
	"errors"
	"fmt"
	"runtime/debug"
	"strings"

	rtasales "github.com/Miku0139oao/rta-sales-client-go"
)

// GetSalesAnalysisItems returns one period's article rows from the last
// analysis kept in this process. RunSalesAnalysis omits those rows so the
// Wails/WebView2 bridge does not marshal a multi-store dump in one call.
func (a *App) GetSalesAnalysisItems(request SalesAnalysisItemsRequest) (SalesAnalysisPackedItems, error) {
	operationID := strings.TrimSpace(request.OperationID)
	periodKey := strings.TrimSpace(request.PeriodKey)
	if operationID == "" || periodKey == "" {
		return SalesAnalysisPackedItems{}, errors.New("operationId and periodKey are required")
	}
	a.salesResultMu.Lock()
	defer a.salesResultMu.Unlock()
	if a.salesResult == nil || a.salesResult.OperationID != operationID {
		return SalesAnalysisPackedItems{}, errors.New("sales analysis result is no longer available")
	}
	packed, ok := a.salesPacked[periodKey]
	if !ok {
		return SalesAnalysisPackedItems{}, fmt.Errorf("period %q was not found", periodKey)
	}
	storeID := strings.TrimSpace(request.StoreID)
	if storeID == "" {
		return packed, nil
	}
	return filterPackedItemsByStore(packed, periodStores(a.salesResult, periodKey), storeID), nil
}

// GetSalesAnalysisReportGlyphs returns the unique characters needed to subset
// the report font without sending article rows to the webview.
func (a *App) GetSalesAnalysisReportGlyphs(request OperationRequest) (string, error) {
	operationID := strings.TrimSpace(request.OperationID)
	a.salesResultMu.Lock()
	defer a.salesResultMu.Unlock()
	if a.salesResult == nil || (operationID != "" && a.salesResult.OperationID != operationID) {
		return "", errors.New("sales analysis result is no longer available")
	}
	seen := make(map[rune]struct{}, 512)
	add := func(value string) {
		for _, character := range value {
			if character != 0 {
				seen[character] = struct{}{}
			}
		}
	}
	add(a.salesResult.From)
	add(a.salesResult.To)
	for _, store := range a.salesResult.Stores {
		add(store.BusinessID)
		add(store.Label)
	}
	for _, period := range a.salesResult.Periods {
		add(period.Key)
		add(period.Label)
		add(period.From)
		add(period.To)
		for _, store := range period.Stores {
			add(store.BusinessID)
			add(store.Label)
		}
	}
	for _, week := range a.salesResult.Weeks {
		add(week.From)
		add(week.To)
		for _, store := range week.Stores {
			add(store.BusinessID)
			add(store.Label)
		}
	}
	for _, packed := range a.salesPacked {
		for _, value := range packed.Dict {
			add(value)
		}
	}
	var builder strings.Builder
	builder.Grow(len(seen))
	for character := range seen {
		builder.WriteRune(character)
	}
	return builder.String(), nil
}

func (a *App) rememberSalesAnalysis(result SalesAnalysisResult, packed map[string]SalesAnalysisPackedItems) SalesAnalysisResult {
	if packed == nil {
		packed = make(map[string]SalesAnalysisPackedItems, len(result.Periods))
		for _, period := range result.Periods {
			packed[period.Key] = packSalesAnalysisItems(period)
		}
	}
	slim := slimSalesAnalysis(result)
	a.salesResultMu.Lock()
	a.salesResult = &slim
	a.salesPacked = packed
	a.salesResultMu.Unlock()
	go debug.FreeOSMemory()
	return slim
}

// ClearSalesAnalysis drops the last analysis cache after the user leaves the
// result screen. Packed rows stay available until this is called so PDF export
// can still reload every period.
func (a *App) ClearSalesAnalysis(request OperationRequest) error {
	operationID := strings.TrimSpace(request.OperationID)
	_ = a.CancelSalesAnalysis(OperationRequest{OperationID: operationID})
	a.salesResultMu.Lock()
	if a.salesResult != nil && (operationID == "" || a.salesResult.OperationID == operationID) {
		a.salesResult = nil
		a.salesPacked = nil
	}
	a.salesResultMu.Unlock()
	go debug.FreeOSMemory()
	return nil
}

type packedPeriodBuilder struct {
	dict       *packedStringDict
	storeIndex map[string]int
	rows       []SalesAnalysisPackedRow
}

func newPackedPeriodBuilder() *packedPeriodBuilder {
	return &packedPeriodBuilder{dict: newPackedStringDict(), storeIndex: make(map[string]int)}
}

func (b *packedPeriodBuilder) appendStore(store rtasales.Store, result *rtasales.SalesResult) (SalesAnalysisTotals, int, error) {
	if result == nil {
		return SalesAnalysisTotals{}, 0, errors.New("RTA returned no sales result")
	}
	index, ok := b.storeIndex[store.BusinessID]
	if !ok {
		index = len(b.storeIndex)
		b.storeIndex[store.BusinessID] = index
	}
	totals := SalesAnalysisTotals{}
	added := 0
	for _, source := range result.Items {
		saleQuantity := countValue(source.TPSaleQuantity)
		returnQuantity := countValue(source.TPReturnSaleQuantity)
		netQuantity := countValue(source.TPGrossSaleQuantity)
		transactionCount := countValue(source.TPTransactionCount)
		returnTransactions := countValue(source.TPReturnTransactionCount)
		if !finite(transactionCount) || !finite(saleQuantity) || !finite(source.TPSaleAmount) ||
			!finite(returnTransactions) || !finite(returnQuantity) || !finite(source.TPReturnSaleAmount) ||
			!finite(netQuantity) || !finite(source.TPGrossSaleAmount) {
			return SalesAnalysisTotals{}, 0, fmt.Errorf("article %q contains a non-finite sales value", source.Matnr)
		}
		b.rows = append(b.rows, SalesAnalysisPackedRow{
			S: index, Ac: b.dict.add(source.Matnr), An: b.dict.add(source.ArticleName), Br: b.dict.add(source.BrandName),
			C1: b.dict.add(source.PurchaseCategory1Name), K1: b.dict.add(source.PurchaseCategory1Code),
			C2: b.dict.add(source.PurchaseCategory2Name), K2: b.dict.add(source.PurchaseCategory2Code),
			C3: b.dict.add(source.PurchaseCategory3Name), K3: b.dict.add(source.PurchaseCategory3Code),
			C4: b.dict.add(source.PurchaseCategory4Name), K4: b.dict.add(source.PurchaseCategory4Code),
			C5: b.dict.add(source.PurchaseCategory5Name), K5: b.dict.add(source.PurchaseCategory5Code),
			T: transactionCount, Sq: saleQuantity, Sa: source.TPSaleAmount,
			Rq: returnQuantity, Rt: returnTransactions, Ra: source.TPReturnSaleAmount,
			Nq: netQuantity, Ns: source.TPGrossSaleAmount,
		})
		added++
		totals.SaleQuantity += saleQuantity
		totals.SaleAmount += source.TPSaleAmount
		totals.ReturnQuantity += returnQuantity
		totals.ReturnAmount += source.TPReturnSaleAmount
		totals.NetQuantity += netQuantity
		totals.NetSalesAmount += source.TPGrossSaleAmount
	}
	if result.TotalTransactionCount != nil {
		if !finite(*result.TotalTransactionCount) {
			return SalesAnalysisTotals{}, 0, errors.New("RTA returned a non-finite transaction count")
		}
		transactionCount := countValue(*result.TotalTransactionCount)
		totals.TransactionCount = &transactionCount
	}
	if result.TrendGrossSaleAmount != nil {
		if !finite(*result.TrendGrossSaleAmount) {
			return SalesAnalysisTotals{}, 0, errors.New("RTA returned a non-finite Trend View net sales amount")
		}
		trendNetSalesAmount := *result.TrendGrossSaleAmount
		totals.TrendNetSalesAmount = &trendNetSalesAmount
	}
	return totals, added, nil
}

func (b *packedPeriodBuilder) finish(periodKey string) SalesAnalysisPackedItems {
	packed := SalesAnalysisPackedItems{PeriodKey: periodKey, Dict: b.dict.list, Rows: b.rows}
	if b.dict != nil {
		b.dict.idx = nil
	}
	b.storeIndex = nil
	b.rows = nil
	return packed
}

func slimSalesAnalysis(full SalesAnalysisResult) SalesAnalysisResult {
	slim := full
	slim.Items = nil
	if len(full.Periods) == 0 {
		return slim
	}
	slim.Periods = make([]SalesAnalysisPeriodResult, len(full.Periods))
	for index, period := range full.Periods {
		if period.ItemCount == 0 {
			period.ItemCount = len(period.Items)
		}
		period.Items = nil
		slim.Periods[index] = period
	}
	return slim
}

func periodStores(result *SalesAnalysisResult, periodKey string) []SalesAnalysisStoreSummary {
	if result == nil {
		return nil
	}
	for _, period := range result.Periods {
		if period.Key == periodKey && len(period.Stores) > 0 {
			return period.Stores
		}
	}
	return result.Stores
}

func filterPackedItemsByStore(packed SalesAnalysisPackedItems, stores []SalesAnalysisStoreSummary, storeID string) SalesAnalysisPackedItems {
	index := -1
	for storeIndex, store := range stores {
		if store.BusinessID == storeID {
			index = storeIndex
			break
		}
	}
	if index < 0 {
		return SalesAnalysisPackedItems{PeriodKey: packed.PeriodKey, Dict: []string{""}}
	}
	remapAt := map[int]int{0: 0}
	dict := []string{""}
	remap := func(old int) int {
		if old <= 0 || old >= len(packed.Dict) {
			return 0
		}
		if next, ok := remapAt[old]; ok {
			return next
		}
		next := len(dict)
		dict = append(dict, packed.Dict[old])
		remapAt[old] = next
		return next
	}
	rows := make([]SalesAnalysisPackedRow, 0)
	for _, row := range packed.Rows {
		if row.S != index {
			continue
		}
		row.S = 0
		row.Ac = remap(row.Ac)
		row.An = remap(row.An)
		row.Br = remap(row.Br)
		row.C1 = remap(row.C1)
		row.K1 = remap(row.K1)
		row.C2 = remap(row.C2)
		row.K2 = remap(row.K2)
		row.C3 = remap(row.C3)
		row.K3 = remap(row.K3)
		row.C4 = remap(row.C4)
		row.K4 = remap(row.K4)
		row.C5 = remap(row.C5)
		row.K5 = remap(row.K5)
		rows = append(rows, row)
	}
	return SalesAnalysisPackedItems{PeriodKey: packed.PeriodKey, Dict: dict, Rows: rows}
}

func packSalesAnalysisItems(period SalesAnalysisPeriodResult) SalesAnalysisPackedItems {
	dict := newPackedStringDict()
	storeIndex := make(map[string]int, len(period.Stores))
	for index, store := range period.Stores {
		storeIndex[store.BusinessID] = index
	}
	rows := make([]SalesAnalysisPackedRow, 0, len(period.Items))
	for _, item := range period.Items {
		index, ok := storeIndex[item.StoreID]
		if !ok {
			index = len(storeIndex)
			storeIndex[item.StoreID] = index
		}
		rows = append(rows, SalesAnalysisPackedRow{
			S: index, Ac: dict.add(item.ArticleCode), An: dict.add(item.ArticleName), Br: dict.add(item.BrandName),
			C1: dict.add(item.Category1), K1: dict.add(item.Category1Code),
			C2: dict.add(item.Category2), K2: dict.add(item.Category2Code),
			C3: dict.add(item.Category3), K3: dict.add(item.Category3Code),
			C4: dict.add(item.Category4), K4: dict.add(item.Category4Code),
			C5: dict.add(item.Category5), K5: dict.add(item.Category5Code),
			T: item.TransactionCount, Sq: item.SaleQuantity, Sa: item.SaleAmount,
			Rq: item.ReturnQuantity, Rt: item.ReturnTransactionCount, Ra: item.ReturnAmount,
			Nq: item.NetQuantity, Ns: item.NetSalesAmount,
		})
	}
	return SalesAnalysisPackedItems{PeriodKey: period.Key, Dict: dict.list, Rows: rows}
}

func unpackSalesAnalysisItems(batch SalesAnalysisPackedItems, stores []SalesAnalysisStoreSummary) []SalesAnalysisItem {
	items := make([]SalesAnalysisItem, 0, len(batch.Rows))
	for _, row := range batch.Rows {
		storeID, storeLabel := "", ""
		if row.S >= 0 && row.S < len(stores) {
			storeID = stores[row.S].BusinessID
			storeLabel = stores[row.S].Label
		}
		items = append(items, SalesAnalysisItem{
			StoreID: storeID, StoreLabel: storeLabel,
			ArticleCode: packedString(batch.Dict, row.Ac), ArticleName: packedString(batch.Dict, row.An), BrandName: packedString(batch.Dict, row.Br),
			Category1: packedString(batch.Dict, row.C1), Category1Code: packedString(batch.Dict, row.K1),
			Category2: packedString(batch.Dict, row.C2), Category2Code: packedString(batch.Dict, row.K2),
			Category3: packedString(batch.Dict, row.C3), Category3Code: packedString(batch.Dict, row.K3),
			Category4: packedString(batch.Dict, row.C4), Category4Code: packedString(batch.Dict, row.K4),
			Category5: packedString(batch.Dict, row.C5), Category5Code: packedString(batch.Dict, row.K5),
			TransactionCount: row.T, SaleQuantity: row.Sq, SaleAmount: row.Sa,
			ReturnQuantity: row.Rq, ReturnTransactionCount: row.Rt, ReturnAmount: row.Ra,
			NetQuantity: row.Nq, NetSalesAmount: row.Ns,
		})
	}
	return items
}

type packedStringDict struct {
	list []string
	idx  map[string]int
}

func newPackedStringDict() *packedStringDict {
	return &packedStringDict{list: []string{""}, idx: map[string]int{"": 0}}
}

func (d *packedStringDict) add(value string) int {
	if value == "" {
		return 0
	}
	if index, ok := d.idx[value]; ok {
		return index
	}
	index := len(d.list)
	d.list = append(d.list, value)
	d.idx[value] = index
	return index
}

func packedString(dict []string, index int) string {
	if index <= 0 || index >= len(dict) {
		return ""
	}
	return dict[index]
}
