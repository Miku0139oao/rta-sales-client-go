package desktop

import (
	"encoding/json"
	"fmt"
	"testing"
)

func syntheticPackedPeriod(itemCount, storeCount, uniqueArticles int) SalesAnalysisPeriodResult {
	if storeCount < 1 {
		storeCount = 1
	}
	if uniqueArticles < 1 {
		uniqueArticles = 1
	}
	stores := make([]SalesAnalysisStoreSummary, storeCount)
	for index := 0; index < storeCount; index++ {
		id := fmt.Sprintf("%d", 100+index)
		stores[index] = SalesAnalysisStoreSummary{BusinessID: id, Label: id + " - Store"}
	}
	items := make([]SalesAnalysisItem, itemCount)
	for index := 0; index < itemCount; index++ {
		store := stores[index%storeCount]
		article := index % uniqueArticles
		quantity := float64((index % 17) + 1)
		price := 10.25 + float64(article%50)
		saleAmount := quantity * price
		items[index] = SalesAnalysisItem{
			StoreID: store.BusinessID, StoreLabel: store.Label,
			Category1: "HEALTH", Category1Code: "A", Category2: "BEAUTY", Category2Code: "A02",
			ArticleCode: fmt.Sprintf("%d", 100000+article), ArticleName: fmt.Sprintf("Item %d", article),
			BrandName: fmt.Sprintf("Brand %d", article%40),
			TransactionCount: quantity, SaleQuantity: quantity, SaleAmount: saleAmount,
			NetQuantity: quantity, NetSalesAmount: saleAmount,
		}
	}
	return SalesAnalysisPeriodResult{Key: "current", Stores: stores, Items: items, ItemCount: itemCount}
}

func checksumItems(items []SalesAnalysisItem) (count int, netSales, netQty float64, codes int) {
	for _, item := range items {
		count++
		netSales += item.NetSalesAmount
		netQty += item.NetQuantity
		codes += len(item.ArticleCode)
	}
	return
}

func TestUnpackPackedJSONShortArrayAndLegacyObject(t *testing.T) {
	stores := []SalesAnalysisStoreSummary{{BusinessID: "107", Label: "107 - Central"}}
	compact, err := unmarshalPackedRow([]byte(`[0,1,2,3,0,0,0,0,0,0,0,0,0,0,2,1.5,10.25,0,0,0,1.5,10.25]`))
	if err != nil {
		t.Fatal(err)
	}
	short, err := unmarshalPackedRow([]byte(`[0,1]`))
	if err != nil {
		t.Fatal(err)
	}
	object, err := unmarshalPackedRow([]byte(`{"s":0,"ac":1,"an":2,"ns":8.5}`))
	if err != nil {
		t.Fatal(err)
	}
	packed := SalesAnalysisPackedItems{
		PeriodKey: "current",
		Dict:      []string{"", "A1", "Mask", "AHC"},
		Rows:      []SalesAnalysisPackedRow{compact, short, object},
	}
	items := unpackSalesAnalysisItems(packed, stores)
	if len(items) != 3 {
		t.Fatalf("len=%d", len(items))
	}
	if items[0].ArticleCode != "A1" || items[0].BrandName != "AHC" || items[0].SaleAmount != 10.25 || items[0].StoreID != "107" {
		t.Fatalf("compact=%#v", items[0])
	}
	if items[1].ArticleCode != "A1" || items[1].ArticleName != "" || items[1].NetSalesAmount != 0 {
		t.Fatalf("short=%#v", items[1])
	}
	if items[2].ArticleName != "Mask" || items[2].NetSalesAmount != 8.5 || items[2].SaleAmount != 0 {
		t.Fatalf("object=%#v", items[2])
	}
	missing := unpackSalesAnalysisItems(SalesAnalysisPackedItems{
		Dict: []string{"", "A1"},
		Rows: []SalesAnalysisPackedRow{{S: 9, Ac: 99, An: -1, Ns: 4.5}},
	}, stores)
	if missing[0].StoreID != "" || missing[0].ArticleCode != "" || missing[0].ArticleName != "" || missing[0].NetSalesAmount != 4.5 {
		t.Fatalf("missing dict/store=%#v", missing[0])
	}
	var aliased SalesAnalysisPackedItems
	if err := json.Unmarshal([]byte(`{"k":"current","d":["","B2","Wipes"],"r":[[0,1,2]]}`), &aliased); err != nil {
		t.Fatal(err)
	}
	fromAlias := unpackSalesAnalysisItems(aliased, stores)
	if fromAlias[0].ArticleCode != "B2" || fromAlias[0].ArticleName != "Wipes" || fromAlias[0].StoreLabel != "107 - Central" {
		t.Fatalf("alias=%#v", fromAlias[0])
	}
}

func TestPackUnpackSyntheticRoundtrip100k(t *testing.T) {
	assertPackedRoundtrip(t, 100000, 40, 2500)
}

func TestPackUnpackSyntheticRoundtrip200k(t *testing.T) {
	assertPackedRoundtrip(t, 200000, 80, 2500)
}

func assertPackedRoundtrip(t *testing.T, itemCount, storeCount, uniqueArticles int) {
	t.Helper()
	period := syntheticPackedPeriod(itemCount, storeCount, uniqueArticles)
	packed := packSalesAnalysisItems(period)
	raw, err := json.Marshal(packed)
	if err != nil {
		t.Fatal(err)
	}
	if len(packed.Rows) != itemCount {
		t.Fatalf("rows=%d want %d", len(packed.Rows), itemCount)
	}
	var wire SalesAnalysisPackedItems
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	unpacked := unpackSalesAnalysisItems(wire, period.Stores)
	wantCount, wantSales, wantQty, wantCodes := checksumItems(period.Items)
	gotCount, gotSales, gotQty, gotCodes := checksumItems(unpacked)
	if gotCount != wantCount || gotSales != wantSales || gotQty != wantQty || gotCodes != wantCodes {
		t.Fatalf("checksum got count=%d sales=%v qty=%v codes=%d want count=%d sales=%v qty=%v codes=%d",
			gotCount, gotSales, gotQty, gotCodes, wantCount, wantSales, wantQty, wantCodes)
	}
	last := itemCount - 1
	if unpacked[0].ArticleCode != period.Items[0].ArticleCode || unpacked[0].StoreID != period.Items[0].StoreID {
		t.Fatalf("first=%#v", unpacked[0])
	}
	if unpacked[last].NetSalesAmount != period.Items[last].NetSalesAmount || unpacked[last].StoreLabel != period.Items[last].StoreLabel {
		t.Fatalf("last=%#v", unpacked[last])
	}
}

func BenchmarkPackSalesAnalysisItems100k(b *testing.B) {
	period := syntheticPackedPeriod(100000, 50, 2500)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		packed := packSalesAnalysisItems(period)
		if len(packed.Rows) != 100000 {
			b.Fatalf("rows=%d", len(packed.Rows))
		}
	}
}

func BenchmarkMarshalPackedItems100k(b *testing.B) {
	packed := packSalesAnalysisItems(syntheticPackedPeriod(100000, 50, 2500))
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		raw, err := json.Marshal(packed)
		if err != nil || len(raw) < 1000 {
			b.Fatalf("marshal err=%v len=%d", err, len(raw))
		}
	}
}

func BenchmarkUnmarshalPackedItems100k(b *testing.B) {
	raw, err := json.Marshal(packSalesAnalysisItems(syntheticPackedPeriod(100000, 50, 2500)))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		var packed SalesAnalysisPackedItems
		if err := json.Unmarshal(raw, &packed); err != nil || len(packed.Rows) != 100000 {
			b.Fatalf("unmarshal err=%v rows=%d", err, len(packed.Rows))
		}
	}
}

func BenchmarkUnpackSalesAnalysisItems100k(b *testing.B) {
	period := syntheticPackedPeriod(100000, 50, 2500)
	packed := packSalesAnalysisItems(period)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		items := unpackSalesAnalysisItems(packed, period.Stores)
		if len(items) != 100000 {
			b.Fatalf("items=%d", len(items))
		}
	}
}

func BenchmarkUnpackSalesAnalysisItems200k(b *testing.B) {
	period := syntheticPackedPeriod(200000, 80, 4000)
	packed := packSalesAnalysisItems(period)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		items := unpackSalesAnalysisItems(packed, period.Stores)
		if len(items) != 200000 {
			b.Fatalf("items=%d", len(items))
		}
	}
}
