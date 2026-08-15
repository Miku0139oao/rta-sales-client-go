package rtasales

import (
	"testing"
	"time"
)

func TestSplitSalesDateRange(t *testing.T) {
	tests := []struct {
		name  string
		start time.Time
		end   time.Time
		want  [][2]string
	}{
		{
			name:  "same day",
			start: time.Date(2026, 6, 25, 23, 59, 0, 0, time.UTC),
			end:   time.Date(2026, 6, 25, 1, 0, 0, 0, time.UTC),
			want:  [][2]string{{"2026-06-25", "2026-06-25"}},
		},
		{
			name:  "exactly 90 inclusive days",
			start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			end:   time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
			want:  [][2]string{{"2026-01-01", "2026-03-31"}},
		},
		{
			name:  "91 inclusive days",
			start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			end:   time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
			want:  [][2]string{{"2026-01-01", "2026-03-31"}, {"2026-04-01", "2026-04-01"}},
		},
		{
			name:  "leap year 90 days",
			start: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			end:   time.Date(2024, 3, 30, 0, 0, 0, 0, time.UTC),
			want:  [][2]string{{"2024-01-01", "2024-03-30"}},
		},
		{
			name:  "leap year 91 days",
			start: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			end:   time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC),
			want:  [][2]string{{"2024-01-01", "2024-03-30"}, {"2024-03-31", "2024-03-31"}},
		},
		{
			name:  "half year",
			start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local),
			end:   time.Date(2026, 6, 30, 0, 0, 0, 0, time.Local),
			want: [][2]string{
				{"2026-01-01", "2026-03-31"},
				{"2026-04-01", "2026-06-29"},
				{"2026-06-30", "2026-06-30"},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := splitSalesDateRange(test.start, test.end)
			if len(got) != len(test.want) {
				t.Fatalf("windows=%d, want %d (%+v)", len(got), len(test.want), formatWindows(got))
			}
			for index, window := range got {
				start := window.start.Format("2006-01-02")
				end := window.end.Format("2006-01-02")
				if start != test.want[index][0] || end != test.want[index][1] {
					t.Fatalf("window %d=%s to %s, want %s to %s", index, start, end, test.want[index][0], test.want[index][1])
				}
			}
		})
	}
}

func TestMergeSaleItemsSumsTheSameArticle(t *testing.T) {
	firstAgg := 2.0
	secondAgg := 5.0
	items := mergeSaleItems([]SaleItem{
		{Matnr: "SKU-1", ArticleName: "A", TPSaleAmount: 10, TPSaleQuantity: 1, TPGrossSaleQuantity: 1, TPGrossSaleAmount: 9, TPTransactionCount: 3, TPTransactionCountAgg: &firstAgg},
		{Matnr: "SKU-2", ArticleName: "B", TPSaleAmount: 4, TPReturnSaleAmount: 1, TPReturnSaleQuantity: 1},
		{Matnr: "SKU-1", BrandName: "Brand", TPSaleAmount: 7, TPSaleQuantity: 2, TPGrossSaleQuantity: 2, TPGrossSaleAmount: 6, TPTransactionCount: 1, TPTransactionCountAgg: &secondAgg, TPReturnTransactionCount: 1},
		{Matnr: "", ArticleName: "loose", TPSaleAmount: 3},
		{Matnr: "", ArticleName: "other loose", TPSaleAmount: 2},
	})
	if len(items) != 4 {
		t.Fatalf("merged count=%d, want 4", len(items))
	}
	if items[0].Matnr != "SKU-1" || items[0].TPSaleAmount != 17 || items[0].TPSaleQuantity != 3 || items[0].TPGrossSaleAmount != 15 || items[0].TPTransactionCount != 4 {
		t.Fatalf("SKU-1 was not summed: %+v", items[0])
	}
	if items[0].ArticleName != "A" || items[0].BrandName != "Brand" {
		t.Fatalf("SKU-1 metadata not preserved: %+v", items[0])
	}
	if items[0].TPTransactionCountAgg == nil || *items[0].TPTransactionCountAgg != 7 {
		t.Fatalf("SKU-1 agg=%v, want 7", items[0].TPTransactionCountAgg)
	}
	if items[1].Matnr != "SKU-2" || items[1].TPSaleAmount != 4 {
		t.Fatalf("SKU-2 changed: %+v", items[1])
	}
	if items[2].ArticleName != "loose" || items[3].ArticleName != "other loose" {
		t.Fatalf("empty-code rows were merged: %+v", items)
	}
}

func TestStartOfISOWeekIsMonday(t *testing.T) {
	tests := []struct {
		value time.Time
		want  string
	}{
		{time.Date(2026, 8, 1, 15, 0, 0, 0, time.UTC), "2026-07-27"},
		{time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), "2026-08-03"},
		{time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC), "2026-07-27"},
		{time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "2025-12-29"},
	}
	for _, test := range tests {
		got := startOfISOWeek(test.value).Format("2006-01-02")
		if got != test.want {
			t.Fatalf("startOfISOWeek(%s)=%s, want %s", test.value.Format("2006-01-02"), got, test.want)
		}
	}
}

func TestAddTrendTotalsTreatsMissingWindowsAsZero(t *testing.T) {
	leftAmount, leftCount := 10.0, 3.0
	merged := addTrendTotals(trendTotals{grossSaleAmount: &leftAmount, transactionCount: &leftCount}, trendTotals{})
	if merged.grossSaleAmount == nil || *merged.grossSaleAmount != 10 || merged.transactionCount == nil || *merged.transactionCount != 3 {
		t.Fatalf("missing right window should keep left totals: %+v", merged)
	}
	if got := addTrendTotals(trendTotals{}, trendTotals{}); got.grossSaleAmount != nil || got.transactionCount != nil {
		t.Fatalf("two empty windows should stay empty: %+v", got)
	}
}

func formatWindows(windows []salesDateWindow) [][2]string {
	result := make([][2]string, len(windows))
	for index, window := range windows {
		result[index] = [2]string{window.start.Format("2006-01-02"), window.end.Format("2006-01-02")}
	}
	return result
}
