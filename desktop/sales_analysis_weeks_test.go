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
