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
	byStore := make([]map[string]rtasales.TrendDay, len(stores))
	for index, store := range stores {
		byStore[index] = trendDayMap(store.days)
	}
	result := make([]SalesAnalysisWeek, 0, len(weeks))
	for _, week := range weeks {
		lwFrom := week.from.AddDate(0, 0, -7)
		lwTo := week.to.AddDate(0, 0, -7)
		folded := SalesAnalysisWeek{
			From:   week.from.Format("2006-01-02"),
			To:     week.to.Format("2006-01-02"),
			Stores: make([]SalesAnalysisWeekStore, 0, len(stores)),
		}
		for index, store := range stores {
			row := weekStoreFromDays(store.store, byStore[index], week.from, week.to, lwFrom, lwTo)
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

// foldSalesAnalysisWeeksForPeriods uses the same current/previous ranges as
// the overview when those ranges are weekday-aligned. Non-aligned comparisons
// still use the original full-week lookback behavior used by month mode.
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
	currentSeries, ok := trendSeriesFromOutcome(currentTrend)
	if !ok {
		return nil
	}
	current := periods[primaryIndex]
	previousIndex := -1
	for index, period := range periods {
		if period.key == "previous" {
			previousIndex = index
			break
		}
	}
	if previousIndex >= 0 {
		if _, aligned := weekdayAlignedComparisonOffset(current.from, current.to, periods[previousIndex].from, periods[previousIndex].to); aligned {
			var previousStores []storeTrendSeries
			if previousIndex < len(outcomes) {
				previousTrend, previousOK := periodTrendOutcome(outcomes[previousIndex], storeCount)
				previousSeries, previousOK := trendSeriesFromOutcome(previousTrend)
				if previousOK {
					previousStores = []storeTrendSeries{previousSeries}
				}
			}
			return foldSalesAnalysisWeeksAligned(
				current.from, current.to,
				periods[previousIndex].from, periods[previousIndex].to,
				[]storeTrendSeries{currentSeries}, previousStores,
			)
		}
	}
	return foldSalesAnalysisWeeks(current.from, current.to, []storeTrendSeries{currentSeries})
}

func trendSeriesFromOutcome(outcome storeOutcome) (storeTrendSeries, bool) {
	if outcome.err != nil || outcome.result == nil {
		return storeTrendSeries{}, false
	}
	return storeTrendSeries{store: outcome.result.Store, days: outcome.result.TrendDays}, true
}

func weekdayAlignedComparisonOffset(currentFrom, currentTo, previousFrom, previousTo time.Time) (int, bool) {
	currentFrom = dateOnly(currentFrom)
	currentTo = dateOnly(currentTo)
	previousFrom = dateOnly(previousFrom)
	previousTo = dateOnly(previousTo)
	if currentTo.Before(currentFrom) || previousTo.Before(previousFrom) {
		return 0, false
	}
	if dateDistance(currentFrom, currentTo) != dateDistance(previousFrom, previousTo) {
		return 0, false
	}
	offset := dateDistance(previousFrom, currentFrom)
	if offset <= 0 || offset%7 != 0 || dateDistance(previousTo, currentTo) != offset {
		return 0, false
	}
	return offset, true
}

func dateDistance(earlier, later time.Time) int {
	earlier = utcDate(earlier)
	later = utcDate(later)
	return int(later.Sub(earlier) / (24 * time.Hour))
}

func utcDate(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func foldSalesAnalysisWeeksAligned(
	currentFrom, currentTo, previousFrom, previousTo time.Time,
	currentStores, previousStores []storeTrendSeries,
) []SalesAnalysisWeek {
	offset, ok := weekdayAlignedComparisonOffset(currentFrom, currentTo, previousFrom, previousTo)
	if !ok || len(currentStores) == 0 {
		return nil
	}
	weeks := overlappingISOWeeks(currentFrom, currentTo)
	if len(weeks) == 0 {
		return nil
	}
	currentMaps := make([]map[string]rtasales.TrendDay, len(currentStores))
	for index, store := range currentStores {
		currentMaps[index] = trendDayMap(store.days)
	}
	previousMaps := make([]map[string]rtasales.TrendDay, len(previousStores))
	for index, store := range previousStores {
		previousMaps[index] = trendDayMap(store.days)
	}
	currentFrom = dateOnly(currentFrom)
	currentTo = dateOnly(currentTo)
	result := make([]SalesAnalysisWeek, 0, len(weeks))
	for _, week := range weeks {
		twFrom := laterDate(week.from, currentFrom)
		twTo := earlierDate(week.to, currentTo)
		if twFrom.After(twTo) {
			continue
		}
		folded := SalesAnalysisWeek{
			From:   week.from.Format("2006-01-02"),
			To:     week.to.Format("2006-01-02"),
			Stores: make([]SalesAnalysisWeekStore, 0, len(currentStores)),
		}
		for index, store := range currentStores {
			var previousDays map[string]rtasales.TrendDay
			if index < len(previousMaps) {
				previousDays = previousMaps[index]
			}
			row := weekStoreFromAlignedDays(store.store, currentMaps[index], previousDays, twFrom, twTo, offset)
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

func weekStoreFromAlignedDays(
	store rtasales.Store,
	currentDays, previousDays map[string]rtasales.TrendDay,
	from, to time.Time,
	offset int,
) SalesAnalysisWeekStore {
	row := SalesAnalysisWeekStore{BusinessID: store.BusinessID, Label: store.Label}
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		weekend := isWeekend(day)
		if point, ok := currentDays[day.Format("2006-01-02")]; ok {
			row.SalesTW += point.GrossSaleAmount
			row.CustomersTW += point.TransactionCount
			if weekend {
				row.WeekendSalesTW += point.GrossSaleAmount
				row.WeekendCustomersTW += point.TransactionCount
			} else {
				row.WeekdaySalesTW += point.GrossSaleAmount
				row.WeekdayCustomersTW += point.TransactionCount
			}
		}
		previousDay := day.AddDate(0, 0, -offset)
		if point, ok := previousDays[previousDay.Format("2006-01-02")]; ok {
			row.SalesLW += point.GrossSaleAmount
			row.CustomersLW += point.TransactionCount
			// Classify the comparison slot by the current weekday. The offset is
			// a whole number of weeks, so the previous date has the same class;
			// using the slot also prevents a prior weekday leaking into weekend.
			if weekend {
				row.WeekendSalesLW += point.GrossSaleAmount
				row.WeekendCustomersLW += point.TransactionCount
			} else {
				row.WeekdaySalesLW += point.GrossSaleAmount
				row.WeekdayCustomersLW += point.TransactionCount
			}
		}
	}
	return row
}

func laterDate(left, right time.Time) time.Time {
	if left.After(right) {
		return left
	}
	return right
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
