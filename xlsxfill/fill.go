// Package xlsxfill fills the two manual daily-input columns in an existing
// sales workbook while preserving the workbook's formulas and formatting.
package xlsxfill

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	rtasales "github.com/Miku0139oao/rta-sales-client-go/rtasales"
	"github.com/xuri/excelize/v2"
)

const (
	DefaultSheetName  = "Dairly"
	defaultMaxQueries = 25
	// DefaultMaxJobs bounds a normal multi-day analysis while still allowing a
	// full workbook month to be processed without per-run tuning.
	DefaultMaxJobs = 2000
	// DefaultConcurrency is the number of independent session lanes queried
	// at once. Jobs that share one cookie still run serially; one account may
	// open one login per store so those stores can proceed in parallel.
	DefaultConcurrency = 160
	// MaximumConcurrency is the user-selectable ceiling for independent
	// session lanes across accounts.
	MaximumConcurrency = 160
)

// SalesProvider is implemented by *rtasales.Client and ProviderRouter. It is
// intentionally small so callers can test workbook filling without network
// requests.
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
	InputPath  string
	OutputPath string
	SheetName  string
	Date       time.Time
	Mapper     StoreMapper
	// AllowedBusinessStoreIDs limits queries to the exact business IDs returned
	// by the authenticated client's Stores method. Empty disables filtering.
	AllowedBusinessStoreIDs []string
	Write                   bool
	Overwrite               bool
	AllowPartial            bool
	MaxQueries              int
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
	Date             string  `json:"date"`
	From             string  `json:"from,omitempty"`
	To               string  `json:"to,omitempty"`
	Sheet            string  `json:"sheet"`
	MatchedRows      int     `json:"matched_rows"`
	SelectedRows     int     `json:"selected_rows"`
	SkippedStoreRows int     `json:"skipped_store_rows"`
	UniqueQueries    int     `json:"unique_queries"`
	CompletedJobs    int     `json:"completed_jobs,omitempty"`
	FailedJobs       int     `json:"failed_jobs,omitempty"`
	StagedCells      int     `json:"staged_cells"`
	UnchangedCells   int     `json:"unchanged_cells"`
	SkippedDataRows  int     `json:"skipped_data_rows"`
	Issues           []Issue `json:"issues,omitempty"`
	Complete         bool    `json:"complete"`
	WroteWorkbook    bool    `json:"wrote_workbook"`
}

// ValidationError means at least one matching row could not be filled safely.
// The caller can inspect the returned Report for non-sensitive row details.
type ValidationError struct {
	IssueCount int
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("workbook fill has %d unresolved issue(s)", e.IssueCount)
}

type cellUpdate struct {
	cell  string
	value float64
}

// Fill selects the mapped stores allowed by the caller, queries one RTA day per
// selected store, and stages values for column L (daily sales) and column AB
// (daily customer/transaction count). Total rows, blank rows, formulas, and
// unrelated cells are never modified.
func Fill(ctx context.Context, provider SalesProvider, request Request) (Report, error) {
	report := Report{Date: request.Date.Format("2006-01-02"), Sheet: request.SheetName}
	if request.SheetName == "" {
		report.Sheet = DefaultSheetName
	}
	if nilSalesProvider(provider) {
		return report, &rtasales.InputError{Field: "provider", Message: "is required"}
	}
	if strings.TrimSpace(request.InputPath) == "" {
		return report, &rtasales.InputError{Field: "InputPath", Message: "is required"}
	}
	if request.Date.IsZero() {
		return report, &rtasales.InputError{Field: "Date", Message: "is required"}
	}
	if request.OnlyRow < 0 || request.OnlyRow == 1 {
		return report, &rtasales.InputError{Field: "OnlyRow", Message: "must be zero or a data row greater than one"}
	}
	if request.Write {
		if _, err := validateDistinctOutput(request.InputPath, request.OutputPath); err != nil {
			return report, err
		}
	}
	maxJobs := request.MaxQueries
	if maxJobs <= 0 {
		maxJobs = defaultMaxQueries
	}
	plan, err := Analyze(ctx, provider, BatchRequest{
		InputPath: request.InputPath, SheetName: request.SheetName,
		From: request.Date, To: request.Date, Mapper: request.Mapper,
		AllowedBusinessStoreIDs: request.AllowedBusinessStoreIDs,
		Overwrite:               request.Overwrite, MaxJobs: maxJobs, Concurrency: 1,
		OnlyRow: request.OnlyRow,
	})
	if err != nil {
		if plan.state != nil {
			return plan.Report, err
		}
		var inputError *rtasales.InputError
		if errors.As(err, &inputError) && inputError.Field == "MaxJobs" {
			return report, &rtasales.InputError{Field: "MaxQueries", Message: inputError.Message}
		}
		return report, err
	}
	plan = legacyCellPartialPlan(plan)
	report = plan.Report
	if len(report.Issues) > 0 && !request.AllowPartial {
		return report, &ValidationError{IssueCount: len(report.Issues)}
	}
	if !request.Write {
		return report, nil
	}
	return Apply(ctx, plan, ApplyRequest{
		OutputPath: request.OutputPath, AllowPartial: request.AllowPartial, ForceRecalculate: true,
	})
}

func legacyCellPartialPlan(plan Plan) Plan {
	state := plan.state
	if state == nil {
		return plan
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	for index := range state.rows {
		row := &state.rows[index]
		if row.status == RowStatusIssue && len(row.static) == 0 && row.queryIssue == "" && len(row.cellIssues) > 0 && len(row.updates) > 0 {
			row.status = RowStatusReady
		}
	}
	return state.snapshotLocked()
}

func normalizeAllowedStoreIDs(values []string) (map[string]struct{}, error) {
	if len(values) == 0 {
		return nil, nil
	}
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, &rtasales.InputError{Field: "AllowedBusinessStoreIDs", Message: "must not contain empty IDs"}
		}
		result[value] = struct{}{}
	}
	return result, nil
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
		return "store_not_authorized"
	case errors.As(err, &auth):
		return "authentication_failed"
	case errors.As(err, &captcha):
		return "captcha_failed"
	case isTransientProtocolError(err):
		// A 200 response containing HTML or truncated JSON is an upstream/session
		// disturbance, not a workbook mapping or permission failure.
		return "upstream_error"
	case isTemporaryQueryError(err):
		// Rate limits and transport timeouts stay retryable query failures even
		// when the same account already listed many authorized stores.
		return "query_failed"
	case errors.As(err, &upstream):
		if upstream.StatusCode == 401 || upstream.StatusCode == 403 {
			return "authentication_failed"
		}
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
