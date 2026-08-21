package desktop

import (
	"testing"
	"time"

	rtasales "github.com/Miku0139oao/rta-sales-client-go/rtasales"
)

func TestOverlappingISOWeeksForAugustMTD(t *testing.T) {
	weeks := overlappingISOWeeks(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC))
	if len(weeks) != 3 {
		t.Fatalf("weeks=%d, want 3", len(weeks))
	}
	got := [][2]string{
		{weeks[0].from.Format("2006-01-02"), weeks[0].to.Format("2006-01-02")},
		{weeks[1].from.Format("2006-01-02"), weeks[1].to.Format("2006-01-02")},
		{weeks[2].from.Format("2006-01-02"), weeks[2].to.Format("2006-01-02")},
	}
	want := [][2]string{{"2026-07-27", "2026-08-02"}, {"2026-08-03", "2026-08-09"}, {"2026-08-10", "2026-08-16"}}
	if got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("weeks=%v, want %v", got, want)
	}
}

func TestFoldSalesAnalysisWeeksComparesFullWeekToPreviousWeek(t *testing.T) {
	days := []rtasales.TrendDay{
		{Date: "2026-07-20", GrossSaleAmount: 10, TransactionCount: 1}, // LW weekday
		{Date: "2026-07-25", GrossSaleAmount: 20, TransactionCount: 2}, // LW Saturday
		{Date: "2026-07-27", GrossSaleAmount: 40, TransactionCount: 4}, // TW Monday (before Aug 1)
		{Date: "2026-08-01", GrossSaleAmount: 80, TransactionCount: 8}, // TW Saturday
		{Date: "2026-08-03", GrossSaleAmount: 16, TransactionCount: 3},
	}
	weeks := foldSalesAnalysisWeeks(
		time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC),
		[]storeTrendSeries{{store: rtasales.Store{BusinessID: "107", Label: "Tai Wai"}, days: days}},
	)
	if len(weeks) != 3 {
		t.Fatalf("weeks=%d", len(weeks))
	}
	first := weeks[0]
	if first.From != "2026-07-27" || first.To != "2026-08-02" {
		t.Fatalf("first week=%s to %s", first.From, first.To)
	}
	row := first.Stores[0]
	if row.SalesTW != 120 || row.SalesLW != 30 {
		t.Fatalf("sales TW/LW=%v/%v, want 120/30", row.SalesTW, row.SalesLW)
	}
	if row.WeekendSalesTW != 80 || row.WeekdaySalesTW != 40 {
		t.Fatalf("TW weekday/weekend=%v/%v, want 40/80", row.WeekdaySalesTW, row.WeekendSalesTW)
	}
	if row.WeekendSalesLW != 20 || row.WeekdaySalesLW != 10 {
		t.Fatalf("LW weekday/weekend=%v/%v, want 10/20", row.WeekdaySalesLW, row.WeekendSalesLW)
	}
	if row.CustomersTW != 12 || row.CustomersLW != 3 {
		t.Fatalf("customers TW/LW=%v/%v, want 12/3", row.CustomersTW, row.CustomersLW)
	}
	if first.Totals.SalesTW != 120 || weeks[1].Stores[0].SalesTW != 16 {
		t.Fatalf("totals or second week unexpected: %+v %+v", first.Totals, weeks[1].Stores[0])
	}
}

func TestFoldSalesAnalysisWeeksForPeriodsKeepsMonthStyleISOWeeksWhenPeriodsAreAligned(t *testing.T) {
	currentDays := []rtasales.TrendDay{
		{Date: "2026-07-25", GrossSaleAmount: 40, TransactionCount: 4}, // last Saturday
		{Date: "2026-07-26", GrossSaleAmount: 20, TransactionCount: 2}, // last Sunday
		{Date: "2026-07-27", GrossSaleAmount: 40, TransactionCount: 4}, // this Monday
		{Date: "2026-08-01", GrossSaleAmount: 80, TransactionCount: 8}, // this Saturday
		{Date: "2026-08-02", GrossSaleAmount: 70, TransactionCount: 7}, // this Sunday
		{Date: "2026-08-03", GrossSaleAmount: 30, TransactionCount: 3}, // next Monday
	}
	previousDays := []rtasales.TrendDay{
		{Date: "2026-07-25", GrossSaleAmount: 4000, TransactionCount: 400},
		{Date: "2026-07-26", GrossSaleAmount: 2000, TransactionCount: 200},
		{Date: "2026-07-27", GrossSaleAmount: 1000, TransactionCount: 100},
	}
	periods := []normalizedSalesAnalysisPeriod{
		{key: "current", from: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), to: time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)},
		{key: "previous", from: time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC), to: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)},
	}
	result := foldSalesAnalysisWeeksForPeriods(
		periods,
		[][]storeOutcome{
			{{}, {result: &rtasales.SalesResult{Store: rtasales.Store{Label: "全部"}, TrendDays: currentDays}}},
			{{}, {result: &rtasales.SalesResult{Store: rtasales.Store{Label: "全部"}, TrendDays: previousDays}}},
		},
		0,
		1,
	)
	if len(result) != 2 {
		t.Fatalf("weeks=%d, want 2", len(result))
	}
	if result[0].From != "2026-07-27" || result[0].To != "2026-08-02" {
		t.Fatalf("first week=%s to %s, want 2026-07-27 to 2026-08-02", result[0].From, result[0].To)
	}
	first := result[0].Totals
	if first.SalesTW != 190 || first.SalesLW != 60 {
		t.Fatalf("first week TW/LW=%v/%v, want 190/60 from current lookback, not previous period", first.SalesTW, first.SalesLW)
	}
	if first.WeekendSalesTW != 150 || first.WeekendSalesLW != 60 {
		t.Fatalf("first week weekend=%v/%v, want 150/60", first.WeekendSalesTW, first.WeekendSalesLW)
	}
	if result[1].From != "2026-08-03" || result[1].To != "2026-08-03" {
		t.Fatalf("open week=%s to %s, want clipped to 2026-08-03", result[1].From, result[1].To)
	}
	second := result[1].Totals
	if second.SalesTW != 30 || second.SalesLW != 40 {
		t.Fatalf("open week TW/LW=%v/%v, want 30/40", second.SalesTW, second.SalesLW)
	}
}

func TestFoldSalesAnalysisWeeksStopsOpenWeekAtQueryEnd(t *testing.T) {
	days := []rtasales.TrendDay{
		{Date: "2026-08-10", GrossSaleAmount: 10, TransactionCount: 1},
		{Date: "2026-08-15", GrossSaleAmount: 50, TransactionCount: 5},
		{Date: "2026-08-16", GrossSaleAmount: 200, TransactionCount: 20},
		{Date: "2026-08-17", GrossSaleAmount: 10, TransactionCount: 1},
		{Date: "2026-08-22", GrossSaleAmount: 5, TransactionCount: 1},
		{Date: "2026-08-23", GrossSaleAmount: 999, TransactionCount: 99},
	}
	weeks := foldSalesAnalysisWeeks(
		time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC),
		[]storeTrendSeries{{store: rtasales.Store{Label: "全部"}, days: days}},
	)
	if len(weeks) != 1 {
		t.Fatalf("weeks=%d, want 1", len(weeks))
	}
	week := weeks[0]
	if week.From != "2026-08-17" || week.To != "2026-08-22" {
		t.Fatalf("week=%s to %s, want 2026-08-17 to 2026-08-22", week.From, week.To)
	}
	row := week.Totals
	if row.SalesTW != 15 || row.SalesLW != 60 {
		t.Fatalf("sales TW/LW=%v/%v, want 15/60 (exclude Sunday)", row.SalesTW, row.SalesLW)
	}
	if row.WeekendSalesTW != 5 || row.WeekendSalesLW != 50 {
		t.Fatalf("weekend TW/LW=%v/%v, want 5/50 (this Saturday vs last Saturday)", row.WeekendSalesTW, row.WeekendSalesLW)
	}
}
