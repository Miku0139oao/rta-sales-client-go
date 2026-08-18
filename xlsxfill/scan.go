package xlsxfill

import (
	"context"
	"fmt"
	"strings"
	"time"

	rtasales "github.com/Miku0139oao/rta-sales-client-go/rtasales"
	"github.com/xuri/excelize/v2"
)

// WorkbookScan is a non-sensitive, network-free workbook summary suitable for
// choosing a sheet/date range before Analyze. DateMin and DateMax describe all
// valid data dates in the selected sheet; counts reflect the requested range.
type WorkbookScan struct {
	Sheets     []string `json:"sheets"`
	Sheet      string   `json:"sheet"`
	DateMin    string   `json:"date_min,omitempty"`
	DateMax    string   `json:"date_max,omitempty"`
	RowCount   int      `json:"row_count"`
	StoreCount int      `json:"store_count"`
	JobCount   int      `json:"job_count"`
	Issues     []Issue  `json:"issues,omitempty"`
}

// ScanWorkbook scans an xlsx file without making any network request. Zero
// From/To values are open bounds, so passing both zero summarizes all dates.
func ScanWorkbook(inputPath, sheetName string, from, to time.Time) (WorkbookScan, error) {
	return ScanWorkbookContext(context.Background(), inputPath, sheetName, from, to)
}

// ScanWorkbookContext is the cancellation-aware form of ScanWorkbook.
func ScanWorkbookContext(ctx context.Context, inputPath, sheetName string, from, to time.Time) (WorkbookScan, error) {
	var scan WorkbookScan
	if err := ctx.Err(); err != nil {
		return scan, err
	}
	inputPath = strings.TrimSpace(inputPath)
	if inputPath == "" {
		return scan, &rtasales.InputError{Field: "InputPath", Message: "is required"}
	}
	if !from.IsZero() && !to.IsZero() && calendarKey(to) < calendarKey(from) {
		return scan, &rtasales.InputError{Field: "To", Message: "must not precede From"}
	}
	if sheetName == "" {
		sheetName = DefaultSheetName
	}
	scan.Sheet = sheetName
	book, err := excelize.OpenFile(inputPath, excelize.Options{RawCellValue: true})
	if err != nil {
		return scan, fmt.Errorf("open workbook: %w", err)
	}
	defer func() { _ = book.Close() }()
	if err := ctx.Err(); err != nil {
		return scan, err
	}
	scan.Sheets = append([]string(nil), book.GetSheetList()...)
	if index, inspectErr := book.GetSheetIndex(sheetName); inspectErr != nil || index == -1 {
		if inspectErr != nil {
			return scan, fmt.Errorf("inspect worksheet: %w", inspectErr)
		}
		return scan, &rtasales.InputError{Field: "SheetName", Message: "worksheet does not exist"}
	}
	if err := ctx.Err(); err != nil {
		return scan, err
	}
	maxRow, err := usedRangeLastRow(book, sheetName)
	if err != nil {
		return scan, err
	}
	if err := ctx.Err(); err != nil {
		return scan, err
	}
	props, err := book.GetWorkbookProps()
	if err != nil {
		return scan, fmt.Errorf("read workbook properties: %w", err)
	}
	use1904 := props.Date1904 != nil && *props.Date1904
	stores := make(map[string]struct{})
	type jobKey struct{ date, store string }
	jobs := make(map[jobKey]struct{})
	issues := make(map[string][]int)
	for row := 2; row <= maxRow; row++ {
		if err := ctx.Err(); err != nil {
			return scan, err
		}
		storeID, err := rawCell(book, sheetName, "C", row)
		if err != nil {
			return scan, err
		}
		dateRaw, err := rawCell(book, sheetName, "F", row)
		if err != nil {
			return scan, err
		}
		label, err := rawCell(book, sheetName, "E", row)
		if err != nil {
			return scan, err
		}
		if strings.EqualFold(strings.TrimSpace(label), "Total") || storeID == "" || dateRaw == "" {
			continue
		}
		date, err := parseWorkbookDate(book, sheetName, row, dateRaw, use1904)
		if err != nil {
			issues["invalid_date"] = append(issues["invalid_date"], row)
			continue
		}
		dateText := date.Format("2006-01-02")
		if scan.DateMin == "" || dateText < scan.DateMin {
			scan.DateMin = dateText
		}
		if scan.DateMax == "" || dateText > scan.DateMax {
			scan.DateMax = dateText
		}
		if !from.IsZero() && calendarKey(date) < calendarKey(from) {
			continue
		}
		if !to.IsZero() && calendarKey(date) > calendarKey(to) {
			continue
		}
		scan.RowCount++
		storeID = strings.TrimSpace(storeID)
		stores[storeID] = struct{}{}
		jobs[jobKey{date: dateText, store: storeID}] = struct{}{}
	}
	scan.StoreCount = len(stores)
	scan.JobCount = len(jobs)
	scan.Issues = sortedIssues(issues)
	return scan, nil
}
