package desktop

import (
	"sort"
	"time"

	rtasales "github.com/Miku0139oao/rta-sales-client-go/rtasales"
)

type storeTrendSeries struct {
	store rtasales.Store
	days  []rtasales.TrendDay
}

func foldSalesAnalysisWeeks(from, to time.Time, stores []storeTrendSeries) []SalesAnalysisWeek {
	weeks := overlappingISOWeeks(from, to)
	if len(weeks) == 0 || len(stores) == 0 {
		return nil
	}
	to = dateOnly(to)
	byStore := make([]map[string]rtasales.TrendDay, len(stores))
	for index, store := range stores {
		byStore[index] = trendDayMap(store.days)
	}
	result := make([]SalesAnalysisWeek, 0, len(weeks))
	for _, week := range weeks {
		twFrom := week.from
		twTo := earlierDate(week.to, to)
		if twFrom.After(twTo) {
			continue
		}
		lwFrom := twFrom.AddDate(0, 0, -7)
		lwTo := twTo.AddDate(0, 0, -7)
		folded := SalesAnalysisWeek{
			From:   twFrom.Format("2006-01-02"),
			To:     twTo.Format("2006-01-02"),
			Stores: make([]SalesAnalysisWeekStore, 0, len(stores)),
		}
		for index, store := range stores {
			row := weekStoreFromDays(store.store, byStore[index], twFrom, twTo, lwFrom, lwTo)
			folded.Stores = append(folded.Stores, row)
			addWeekStoreTotals(&folded.Totals, row)
		}
		sort.SliceStable(folded.Stores, func(left, right int) bool {
			if folded.Stores[left].SalesTW == folded.Stores[right].SalesTW {
				return folded.Stores[left].BusinessID < folded.Stores[right].BusinessID
			}
			return folded.Stores[left].SalesTW > folded.Stores[right].SalesTW
		})
		result = append(result, folded)
	}
	return result
}

// foldSalesAnalysisWeeksForPeriods always uses month-mode ISO weeks: each
// Monday–Sunday overlapping the current range versus the same weekdays last
// week. 以星期比較 only changes overview/previous periods, not this tab.
func foldSalesAnalysisWeeksForPeriods(
	periods []normalizedSalesAnalysisPeriod,
	outcomes [][]storeOutcome,
	primaryIndex, storeCount int,
) []SalesAnalysisWeek {
	if primaryIndex < 0 || primaryIndex >= len(periods) {
		return nil
	}
	if primaryIndex >= len(outcomes) {
		return nil
	}
	currentTrend, ok := periodTrendOutcome(outcomes[primaryIndex], storeCount)
	if !ok {
		return nil
	}
	currentSeries, ok := trendSeriesFromOutcome(currentTrend)
	if !ok {
		return nil
	}
	current := periods[primaryIndex]
	return foldSalesAnalysisWeeks(current.from, current.to, []storeTrendSeries{currentSeries})
}

func trendSeriesFromOutcome(outcome storeOutcome) (storeTrendSeries, bool) {
	if outcome.err != nil || outcome.result == nil {
		return storeTrendSeries{}, false
	}
	return storeTrendSeries{store: outcome.result.Store, days: outcome.result.TrendDays}, true
}

func earlierDate(left, right time.Time) time.Time {
	if left.Before(right) {
		return left
	}
	return right
}

type isoWeek struct {
	from time.Time
	to   time.Time
}

func overlappingISOWeeks(from, to time.Time) []isoWeek {
	from = dateOnly(from)
	to = dateOnly(to)
	if to.Before(from) {
		return nil
	}
	start := startOfMonday(from)
	weeks := make([]isoWeek, 0, 6)
	for !start.After(to) {
		end := start.AddDate(0, 0, 6)
		weeks = append(weeks, isoWeek{from: start, to: end})
		start = start.AddDate(0, 0, 7)
	}
	return weeks
}

func weekStoreFromDays(store rtasales.Store, days map[string]rtasales.TrendDay, twFrom, twTo, lwFrom, lwTo time.Time) SalesAnalysisWeekStore {
	row := SalesAnalysisWeekStore{BusinessID: store.BusinessID, Label: store.Label}
	addRange := func(from, to time.Time, salesTW, customersTW, weekdaySales, weekendSales, weekdayCustomers, weekendCustomers *float64) {
		for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
			point, ok := days[day.Format("2006-01-02")]
			if !ok {
				continue
			}
			*salesTW += point.GrossSaleAmount
			*customersTW += point.TransactionCount
			if isWeekend(day) {
				*weekendSales += point.GrossSaleAmount
				*weekendCustomers += point.TransactionCount
			} else {
				*weekdaySales += point.GrossSaleAmount
				*weekdayCustomers += point.TransactionCount
			}
		}
	}
	addRange(twFrom, twTo, &row.SalesTW, &row.CustomersTW, &row.WeekdaySalesTW, &row.WeekendSalesTW, &row.WeekdayCustomersTW, &row.WeekendCustomersTW)
	addRange(lwFrom, lwTo, &row.SalesLW, &row.CustomersLW, &row.WeekdaySalesLW, &row.WeekendSalesLW, &row.WeekdayCustomersLW, &row.WeekendCustomersLW)
	return row
}

func addWeekStoreTotals(destination *SalesAnalysisWeekStore, source SalesAnalysisWeekStore) {
	destination.SalesTW += source.SalesTW
	destination.SalesLW += source.SalesLW
	destination.CustomersTW += source.CustomersTW
	destination.CustomersLW += source.CustomersLW
	destination.WeekdaySalesTW += source.WeekdaySalesTW
	destination.WeekdaySalesLW += source.WeekdaySalesLW
	destination.WeekendSalesTW += source.WeekendSalesTW
	destination.WeekendSalesLW += source.WeekendSalesLW
	destination.WeekdayCustomersTW += source.WeekdayCustomersTW
	destination.WeekdayCustomersLW += source.WeekdayCustomersLW
	destination.WeekendCustomersTW += source.WeekendCustomersTW
	destination.WeekendCustomersLW += source.WeekendCustomersLW
}

func trendDayMap(days []rtasales.TrendDay) map[string]rtasales.TrendDay {
	result := make(map[string]rtasales.TrendDay, len(days))
	for _, day := range days {
		existing := result[day.Date]
		existing.Date = day.Date
		existing.GrossSaleAmount += day.GrossSaleAmount
		existing.TransactionCount += day.TransactionCount
		result[day.Date] = existing
	}
	return result
}

func startOfMonday(value time.Time) time.Time {
	value = dateOnly(value)
	weekday := int(value.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return value.AddDate(0, 0, -(weekday - 1))
}

func dateOnly(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}

func isWeekend(value time.Time) bool {
	weekday := value.Weekday()
	return weekday == time.Saturday || weekday == time.Sunday
}
