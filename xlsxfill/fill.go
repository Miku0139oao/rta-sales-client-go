// Package xlsxfill fills the two manual daily-input columns in an existing
// sales workbook while preserving the workbook's formulas and formatting.
package xlsxfill

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	rtasales "github.com/Miku0139oao/rta-sales-client-go"
	"github.com/xuri/excelize/v2"
)

const (
	DefaultSheetName  = "Dairly"
	defaultMaxQueries = 25
)

// SalesProvider is implemented by *rtasales.Client and is intentionally small
// so callers can test workbook filling without making network requests.
type SalesProvider interface {
	Sales(context.Context, rtasales.SalesQuery) (*rtasales.SalesResult, error)
}

// StoreMapper resolves a workbook-facing store code to the business store ID
// accepted by rtasales.Client.
type StoreMapper interface {
	ResolveStore(workbookStoreID string) (businessStoreID string, ok bool)
}

// Request describes one date-scoped fill operation. Write is false by default,
// making the operation a dry run. OutputPath must differ from InputPath.
type Request struct {
	InputPath    string
	OutputPath   string
	SheetName    string
	Date         time.Time
	Mapper       StoreMapper
	Write        bool
	Overwrite    bool
	AllowPartial bool
	MaxQueries   int
	// OnlyRow restricts the operation to one worksheet row for a bounded test.
	// Zero processes every row matching Date.
	OnlyRow int
}

// Issue identifies rows that were not safe to update without exposing store
// IDs, account names, or returned sales values.
type Issue struct {
	Code string `json:"code"`
	Rows []int  `json:"rows"`
}

// Report summarizes a fill operation without embedding private mapping data or
// sales figures. StagedCells is the number of cells that would be written.
type Report struct {
	Date            string  `json:"date"`
	Sheet           string  `json:"sheet"`
	MatchedRows     int     `json:"matched_rows"`
	UniqueQueries   int     `json:"unique_queries"`
	StagedCells     int     `json:"staged_cells"`
	UnchangedCells  int     `json:"unchanged_cells"`
	SkippedDataRows int     `json:"skipped_data_rows"`
	Issues          []Issue `json:"issues,omitempty"`
	WroteWorkbook   bool    `json:"wrote_workbook"`
}

// ValidationError means at least one matching row could not be filled safely.
// The caller can inspect the returned Report for non-sensitive row details.
type ValidationError struct {
	IssueCount int
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("workbook fill has %d unresolved issue(s)", e.IssueCount)
}

type rowTarget struct {
	row int
}

type cellUpdate struct {
	cell  string
	value float64
}

// Fill queries one RTA day per mapped store and stages values for column L
// (daily sales) and column AB (daily customer/transaction count). Total rows,
// blank rows, formulas, and unrelated cells are never modified.
func Fill(ctx context.Context, provider SalesProvider, request Request) (Report, error) {
	report := Report{Date: request.Date.Format("2006-01-02")}
	if provider == nil {
		return report, &rtasales.InputError{Field: "provider", Message: "is required"}
	}
	if request.Mapper == nil {
		request.Mapper = IdentityStoreMap{}
	}
	request.InputPath = strings.TrimSpace(request.InputPath)
	if request.InputPath == "" {
		return report, &rtasales.InputError{Field: "InputPath", Message: "is required"}
	}
	if request.Date.IsZero() {
		return report, &rtasales.InputError{Field: "Date", Message: "is required"}
	}
	if request.OnlyRow < 0 || request.OnlyRow == 1 {
		return report, &rtasales.InputError{Field: "OnlyRow", Message: "must be zero or a data row greater than one"}
	}
	if request.SheetName == "" {
		request.SheetName = DefaultSheetName
	}
	report.Sheet = request.SheetName
	if request.MaxQueries <= 0 {
		request.MaxQueries = defaultMaxQueries
	}
	if request.Write {
		request.OutputPath = strings.TrimSpace(request.OutputPath)
		if request.OutputPath == "" {
			return report, &rtasales.InputError{Field: "OutputPath", Message: "is required when Write is true"}
		}
		inputAbs, err := filepath.Abs(request.InputPath)
		if err != nil {
			return report, fmt.Errorf("resolve input path: %w", err)
		}
		outputAbs, err := filepath.Abs(request.OutputPath)
		if err != nil {
			return report, fmt.Errorf("resolve output path: %w", err)
		}
		if strings.EqualFold(filepath.Clean(inputAbs), filepath.Clean(outputAbs)) {
			return report, &rtasales.InputError{Field: "OutputPath", Message: "must differ from InputPath"}
		}
	}

	book, err := excelize.OpenFile(request.InputPath, excelize.Options{RawCellValue: true})
	if err != nil {
		return report, fmt.Errorf("open workbook: %w", err)
	}
	defer func() { _ = book.Close() }()
	if index, err := book.GetSheetIndex(request.SheetName); err != nil || index == -1 {
		if err != nil {
			return report, fmt.Errorf("inspect worksheet: %w", err)
		}
		return report, &rtasales.InputError{Field: "SheetName", Message: "worksheet does not exist"}
	}
	maxRow, err := usedRangeLastRow(book, request.SheetName)
	if err != nil {
		return report, err
	}
	props, err := book.GetWorkbookProps()
	if err != nil {
		return report, fmt.Errorf("read workbook properties: %w", err)
	}
	use1904 := props.Date1904 != nil && *props.Date1904

	targetsByStore := make(map[string][]rowTarget)
	issues := make(map[string][]int)
	for row := 2; row <= maxRow; row++ {
		if request.OnlyRow > 0 && row != request.OnlyRow {
			continue
		}
		storeID, err := rawCell(book, request.SheetName, "C", row)
		if err != nil {
			return report, err
		}
		dateRaw, err := rawCell(book, request.SheetName, "F", row)
		if err != nil {
			return report, err
		}
		label, err := rawCell(book, request.SheetName, "E", row)
		if err != nil {
			return report, err
		}
		if strings.EqualFold(strings.TrimSpace(label), "Total") || strings.TrimSpace(storeID) == "" || strings.TrimSpace(dateRaw) == "" {
			report.SkippedDataRows++
			continue
		}
		rowDate, err := parseWorkbookDate(book, request.SheetName, row, dateRaw, use1904)
		if err != nil {
			issues["invalid_date"] = append(issues["invalid_date"], row)
			continue
		}
		if !sameCalendarDate(rowDate, request.Date) {
			continue
		}
		report.MatchedRows++
		businessStoreID, ok := request.Mapper.ResolveStore(strings.TrimSpace(storeID))
		if !ok || strings.TrimSpace(businessStoreID) == "" {
			issues["missing_mapping"] = append(issues["missing_mapping"], row)
			continue
		}
		businessStoreID = strings.TrimSpace(businessStoreID)
		targetsByStore[businessStoreID] = append(targetsByStore[businessStoreID], rowTarget{row: row})
	}
	report.UniqueQueries = len(targetsByStore)
	if report.UniqueQueries > request.MaxQueries {
		return report, &rtasales.InputError{Field: "MaxQueries", Message: fmt.Sprintf("matched %d stores, limit is %d", report.UniqueQueries, request.MaxQueries)}
	}

	storeIDs := make([]string, 0, len(targetsByStore))
	for storeID := range targetsByStore {
		storeIDs = append(storeIDs, storeID)
	}
	sort.Slice(storeIDs, func(i, j int) bool {
		return targetsByStore[storeIDs[i]][0].row < targetsByStore[storeIDs[j]][0].row
	})
	updates := make([]cellUpdate, 0, report.MatchedRows*2)
	for _, storeID := range storeIDs {
		rows := targetsByStore[storeID]
		result, queryErr := provider.Sales(ctx, rtasales.SalesQuery{
			BusinessStoreID: storeID,
			StartDate:       request.Date,
			EndDate:         request.Date,
		})
		if queryErr != nil {
			code := classifyQueryError(queryErr)
			issues[code] = appendRows(issues[code], rows)
			continue
		}
		if result == nil || len(result.Items) == 0 {
			issues["no_data"] = appendRows(issues["no_data"], rows)
			continue
		}
		if math.IsNaN(result.TotalAmount) || math.IsInf(result.TotalAmount, 0) {
			issues["invalid_sales_total"] = appendRows(issues["invalid_sales_total"], rows)
			continue
		}
		if result.TotalTransactionCount == nil || math.IsNaN(*result.TotalTransactionCount) || math.IsInf(*result.TotalTransactionCount, 0) {
			issues["transaction_total_unavailable"] = appendRows(issues["transaction_total_unavailable"], rows)
			continue
		}
		transactionCount := *result.TotalTransactionCount
		if math.Abs(transactionCount-math.Round(transactionCount)) > 1e-9 || transactionCount < 0 {
			issues["invalid_transaction_total"] = appendRows(issues["invalid_transaction_total"], rows)
			continue
		}
		for _, target := range rows {
			rowUpdates, rowIssues, unchanged, err := stageRow(book, request.SheetName, target.row, result.TotalAmount, math.Round(transactionCount), request.Overwrite)
			if err != nil {
				return report, err
			}
			updates = append(updates, rowUpdates...)
			report.UnchangedCells += unchanged
			for _, code := range rowIssues {
				issues[code] = append(issues[code], target.row)
			}
		}
	}

	report.StagedCells = len(updates)
	report.Issues = sortedIssues(issues)
	if len(report.Issues) > 0 && !request.AllowPartial {
		return report, &ValidationError{IssueCount: len(report.Issues)}
	}
	if !request.Write {
		return report, nil
	}
	for _, update := range updates {
		if err := book.SetCellValue(request.SheetName, update.cell, update.value); err != nil {
			return report, fmt.Errorf("write worksheet cell: %w", err)
		}
	}
	auto := "auto"
	yes := true
	if err := book.SetCalcProps(&excelize.CalcPropsOptions{
		CalcMode:       &auto,
		FullCalcOnLoad: &yes,
		ForceFullCalc:  &yes,
		CalcOnSave:     &yes,
	}); err != nil {
		return report, fmt.Errorf("set workbook calculation mode: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(request.OutputPath), 0o755); err != nil && filepath.Dir(request.OutputPath) != "." {
		return report, fmt.Errorf("create output directory: %w", err)
	}
	if err := book.SaveAs(request.OutputPath); err != nil {
		return report, fmt.Errorf("save output workbook: %w", err)
	}
	report.WroteWorkbook = true
	return report, nil
}

func usedRangeLastRow(book *excelize.File, sheet string) (int, error) {
	dimension, err := book.GetSheetDimension(sheet)
	if err != nil {
		return 0, fmt.Errorf("read worksheet dimension: %w", err)
	}
	parts := strings.Split(dimension, ":")
	last := parts[len(parts)-1]
	_, row, err := excelize.CellNameToCoordinates(last)
	if err != nil {
		return 0, fmt.Errorf("parse worksheet dimension: %w", err)
	}
	rows, err := book.GetRows(sheet, excelize.Options{RawCellValue: true})
	if err != nil {
		return 0, fmt.Errorf("read worksheet rows: %w", err)
	}
	if len(rows) > row {
		row = len(rows)
	}
	return row, nil
}

func rawCell(book *excelize.File, sheet, column string, row int) (string, error) {
	value, err := book.GetCellValue(sheet, column+strconv.Itoa(row), excelize.Options{RawCellValue: true})
	if err != nil {
		return "", fmt.Errorf("read worksheet cell: %w", err)
	}
	return strings.TrimSpace(value), nil
}

func parseWorkbookDate(book *excelize.File, sheet string, row int, raw string, use1904 bool) (time.Time, error) {
	if serial, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil {
		return excelize.ExcelDateToTime(serial, use1904)
	}
	formatted, err := book.GetCellValue(sheet, "F"+strconv.Itoa(row))
	if err != nil {
		return time.Time{}, err
	}
	for _, candidate := range []string{raw, formatted} {
		for _, layout := range []string{"2006-01-02", "02/01/2006", "2/1/2006", "01/02/2006", "1/2/2006"} {
			if parsed, parseErr := time.ParseInLocation(layout, strings.TrimSpace(candidate), time.Local); parseErr == nil {
				return parsed, nil
			}
		}
	}
	return time.Time{}, errors.New("unsupported workbook date")
}

func sameCalendarDate(left, right time.Time) bool {
	ly, lm, ld := left.Date()
	ry, rm, rd := right.Date()
	return ly == ry && lm == rm && ld == rd
}

func appendRows(existing []int, targets []rowTarget) []int {
	for _, target := range targets {
		existing = append(existing, target.row)
	}
	return existing
}

func stageRow(book *excelize.File, sheet string, row int, salesAmount, transactionCount float64, overwrite bool) ([]cellUpdate, []string, int, error) {
	updates := make([]cellUpdate, 0, 2)
	issues := make([]string, 0, 2)
	unchanged := 0
	for _, target := range []struct {
		column string
		value  float64
	}{
		{column: "L", value: salesAmount},
		{column: "AB", value: transactionCount},
	} {
		cell := target.column + strconv.Itoa(row)
		formula, err := book.GetCellFormula(sheet, cell)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("inspect target formula: %w", err)
		}
		if strings.TrimSpace(formula) != "" {
			issues = append(issues, "target_contains_formula")
			continue
		}
		existing, err := book.GetCellValue(sheet, cell, excelize.Options{RawCellValue: true})
		if err != nil {
			return nil, nil, 0, fmt.Errorf("read target value: %w", err)
		}
		existing = strings.TrimSpace(existing)
		if existing != "" {
			if current, parseErr := strconv.ParseFloat(existing, 64); parseErr == nil && nearlyEqual(current, target.value) {
				unchanged++
				continue
			}
			if !overwrite {
				issues = append(issues, "existing_value_differs")
				continue
			}
		}
		updates = append(updates, cellUpdate{cell: cell, value: target.value})
	}
	return updates, issues, unchanged, nil
}

func nearlyEqual(left, right float64) bool {
	tolerance := math.Max(0.005, math.Max(math.Abs(left), math.Abs(right))*1e-10)
	return math.Abs(left-right) <= tolerance
}

func classifyQueryError(err error) string {
	var notFound *rtasales.StoreNotFoundError
	var auth *rtasales.AuthError
	var captcha *rtasales.CaptchaError
	var upstream *rtasales.UpstreamError
	switch {
	case errors.As(err, &notFound):
		return "store_not_accessible"
	case errors.As(err, &auth):
		return "authentication_failed"
	case errors.As(err, &captcha):
		return "captcha_failed"
	case errors.As(err, &upstream):
		return "upstream_error"
	default:
		return "query_failed"
	}
}

func sortedIssues(input map[string][]int) []Issue {
	codes := make([]string, 0, len(input))
	for code, rows := range input {
		if len(rows) > 0 {
			codes = append(codes, code)
		}
	}
	sort.Strings(codes)
	issues := make([]Issue, 0, len(codes))
	for _, code := range codes {
		rows := append([]int(nil), input[code]...)
		sort.Ints(rows)
		rows = uniqueInts(rows)
		issues = append(issues, Issue{Code: code, Rows: rows})
	}
	return issues
}

func uniqueInts(values []int) []int {
	if len(values) < 2 {
		return values
	}
	result := values[:1]
	for _, value := range values[1:] {
		if value != result[len(result)-1] {
			result = append(result, value)
		}
	}
	return result
}
