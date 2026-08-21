package xlsxfill

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	rtasales "github.com/Miku0139oao/rta-sales-client-go/rtasales"
	"github.com/xuri/excelize/v2"
)

var defaultRetryDelays = [...]time.Duration{time.Second, 3 * time.Second}

// BackoffFunc waits between retry attempts. Tests and embedding applications
// can inject a clock-aware implementation; the default honors cancellation.
type BackoffFunc func(context.Context, time.Duration) error

// ProgressFunc receives one non-sensitive event after each query job reaches a
// terminal state. Callbacks are invoked serially by Analyze or RetryFailed.
type ProgressFunc func(ProgressEvent)

// ProgressEvent keeps routing details in memory for trusted callers, while its
// JSON form omits profiles, store IDs, and sales data.
type ProgressEvent struct {
	Stage         string `json:"stage"`
	CompletedJobs int    `json:"completed_jobs"`
	TotalJobs     int    `json:"total_jobs"`
	Attempt       int    `json:"attempt"`
	Date          string `json:"date"`
	Profile       string `json:"-"`
	StoreID       string `json:"-"`
	Status        string `json:"status"`
}

// BatchRequest configures an inclusive multi-day workbook analysis. From and
// To are compared as calendar dates. MaxJobs defaults to 2000, and Concurrency
// defaults to 160 account sessions with a hard maximum of 160.
type BatchRequest struct {
	InputPath               string
	SheetName               string
	From                    time.Time
	To                      time.Time
	Mapper                  StoreMapper
	AllowedBusinessStoreIDs []string
	Overwrite               bool
	MaxJobs                 int
	Concurrency             int
	// OnlyRow is diagnostic-only. Zero scans every worksheet data row.
	OnlyRow int
	// Progress is the preferred callback name. ProgressCallback is retained as
	// a descriptive alias; setting both is invalid.
	Progress         ProgressFunc
	ProgressCallback ProgressFunc
	// Backoff replaces the cancellation-aware retry wait. It is primarily useful
	// for deterministic tests; production callers should normally leave it nil.
	Backoff BackoffFunc
}

// ApplyRequest controls the mutation phase. OutputPath must not identify the
// source workbook. AllowPartial is required when the plan has any issues.
type ApplyRequest struct {
	OutputPath       string
	AllowPartial     bool
	ForceRecalculate bool
}

// SourceFingerprint binds a plan to the exact source workbook it analyzed.
type SourceFingerprint struct {
	SHA256  string
	Size    int64
	ModTime time.Time
}

// CurrentValues records the raw source values and formulas for the two manual
// cells. Formula fields are populated even when a cached value also exists.
type CurrentValues struct {
	DailySales              string
	TransactionCount        string
	DailySalesFormula       string
	TransactionCountFormula string
}

// ProposedValues records the Trend View daily gross sales and transaction
// aggregates returned for a row. Nil means no safe proposal exists.
type ProposedValues struct {
	DailySales       *float64
	TransactionCount *float64
}

// RowStatus describes whether Apply may update a planned row.
type RowStatus string

const (
	RowStatusPending   RowStatus = "pending"
	RowStatusReady     RowStatus = "ready"
	RowStatusUnchanged RowStatus = "unchanged"
	RowStatusIssue     RowStatus = "issue"
	RowStatusSkipped   RowStatus = "skipped"
)

// RowPlan is an in-memory audit record. Its current/proposed values, route
// profile, and date are deliberately excluded from JSON to avoid accidental
// disclosure through generic logging.
type RowPlan struct {
	Row             int            `json:"row"`
	Date            string         `json:"-"`
	WorkbookStoreID string         `json:"-"`
	Current         CurrentValues  `json:"-"`
	Proposed        ProposedValues `json:"-"`
	Status          RowStatus      `json:"status"`
	Profile         string         `json:"-"`
	Issues          []string       `json:"issues,omitempty"`
}

// Plan is the immutable public snapshot returned by Analyze. Operational
// state (providers, mapped store IDs, cell values, and source path) remains in
// memory and cannot be serialized. Pass Plan by value to Apply/RetryFailed.
type Plan struct {
	InputPath string            `json:"-"`
	Sheet     string            `json:"sheet"`
	From      time.Time         `json:"-"`
	To        time.Time         `json:"-"`
	Source    SourceFingerprint `json:"-"`
	Rows      []RowPlan         `json:"-"`
	Issues    []Issue           `json:"issues,omitempty"`
	Complete  bool              `json:"complete"`
	Report    Report            `json:"report"`

	state *batchPlanState
}

// MarshalJSON emits only the non-sensitive aggregate report. In particular it
// never serializes source paths, hashes, providers, store IDs, profiles, or
// current/proposed sales figures.
func (plan Plan) MarshalJSON() ([]byte, error) {
	type safePlan struct {
		Complete bool   `json:"complete"`
		Report   Report `json:"report"`
	}
	return json.Marshal(safePlan{Complete: plan.Complete, Report: plan.Report})
}

// String returns the same redacted representation as MarshalJSON so ordinary
// structured logging does not print row values or route metadata.
func (plan Plan) String() string {
	encoded, err := plan.MarshalJSON()
	if err != nil {
		return `{"complete":false,"report":{}}`
	}
	return string(encoded)
}

// IsComplete reports the canonical state, including updates made by a returned
// RetryFailed plan that shares the same in-memory state.
func (plan Plan) IsComplete() bool {
	if plan.state == nil {
		return false
	}
	plan.state.mu.Lock()
	defer plan.state.mu.Unlock()
	return plan.state.complete
}

// IncompletePlanError is returned when Apply is attempted while analysis still
// has pending jobs, most commonly after cancellation.
type IncompletePlanError struct{}

func (*IncompletePlanError) Error() string { return "workbook analysis plan is incomplete" }

// PlanBusyError is returned when a callback or another goroutine attempts to
// apply or retry a plan while its query runner is still active.
type PlanBusyError struct{}

func (*PlanBusyError) Error() string { return "workbook analysis plan is busy" }

// SourceChangedError prevents applying results to a workbook that is no longer
// byte-for-byte and metadata-identical to the analyzed source.
type SourceChangedError struct {
	Expected SourceFingerprint
	Actual   SourceFingerprint
}

func (*SourceChangedError) Error() string { return "source workbook changed after analysis" }

type batchPlanState struct {
	mu sync.Mutex
	// executing is true while Analyze or RetryFailed owns the query runner.
	// Progress callbacks run without mu held, so re-entrant Apply/RetryFailed
	// calls must fail closed instead of mutating the live plan.
	executing bool

	resumeProvider SalesProvider
	resumeRequest  BatchRequest
	scanComplete   bool

	inputPath   string
	sheet       string
	from        time.Time
	to          time.Time
	fingerprint SourceFingerprint
	overwrite   bool
	concurrency int
	progress    ProgressFunc
	backoff     BackoffFunc

	rows        []batchRow
	jobs        []batchJob
	extraIssues map[string][]int
	report      Report
	complete    bool
}

type batchRow struct {
	number          int
	date            string
	workbookStoreID string
	profile         string
	current         CurrentValues
	proposed        ProposedValues
	status          RowStatus
	static          []string
	queryIssue      string
	cellIssues      []string
	updates         []cellUpdate
	unchanged       int
	jobIndex        int
}

type batchJobStatus uint8

const (
	batchJobPending batchJobStatus = iota
	batchJobSucceeded
	batchJobTerminalIssue
	batchJobRetryableFailure
)

type batchJob struct {
	storeID    string
	date       time.Time
	dateText   string
	profile    string
	lane       string
	provider   SalesProvider
	rowIndexes []int
	status     batchJobStatus
	issueCode  string
	attempts   int
}

type batchJobResult struct {
	index     int
	status    batchJobStatus
	issueCode string
	result    *rtasales.SalesResult
	attempts  int
	cancelled bool
	err       error
}

// Analyze scans all relevant worksheet rows, deduplicates date/store jobs,
// queries each job with a single-day SalesQuery, and returns an in-memory plan.
// Query and row issues are recorded in the plan rather than returned as errors.
// A context error returns the partial, incomplete plan so RetryFailed can resume
// it later.
func Analyze(ctx context.Context, provider SalesProvider, request BatchRequest) (Plan, error) {
	state, err := prepareBatchPlan(ctx, provider, request)
	if err != nil {
		if state != nil {
			state.mu.Lock()
			plan := state.snapshotLocked()
			state.mu.Unlock()
			return plan, err
		}
		return Plan{}, err
	}
	state.mu.Lock()
	err = state.executeJobsLocked(ctx, allJobIndexes(state.jobs))
	plan := state.snapshotLocked()
	state.mu.Unlock()
	return plan, err
}

// RetryFailed re-runs jobs that ended in a retryable transport/upstream error,
// plus jobs left pending by cancellation. Static mapping, authorization,
// no-data, permission, and workbook-format issues are never queried again.
func RetryFailed(ctx context.Context, plan Plan) (Plan, error) {
	if plan.state == nil {
		return Plan{}, &rtasales.InputError{Field: "Plan", Message: "was not produced by Analyze"}
	}
	state := plan.state
	state.mu.Lock()
	if state.executing {
		busy := state.snapshotLocked()
		state.mu.Unlock()
		return busy, &PlanBusyError{}
	}
	if !state.scanComplete {
		provider := state.resumeProvider
		request := state.resumeRequest
		state.mu.Unlock()
		return Analyze(ctx, provider, request)
	}
	indexes := make([]int, 0)
	for index := range state.jobs {
		if state.jobs[index].status == batchJobPending || state.jobs[index].status == batchJobRetryableFailure {
			indexes = append(indexes, index)
			state.jobs[index].status = batchJobPending
			state.jobs[index].issueCode = ""
			for _, rowIndex := range state.jobs[index].rowIndexes {
				state.rows[rowIndex].queryIssue = ""
				state.rows[rowIndex].proposed = ProposedValues{}
				state.rows[rowIndex].cellIssues = nil
				state.rows[rowIndex].updates = nil
				state.rows[rowIndex].unchanged = 0
				state.rows[rowIndex].status = RowStatusPending
			}
		}
	}
	if len(indexes) > 0 {
		state.complete = false
	}
	err := state.executeJobsLocked(ctx, indexes)
	retried := state.snapshotLocked()
	state.mu.Unlock()
	return retried, err
}

// RetryFailed is also available as a Plan method for fluent callers.
func (plan Plan) RetryFailed(ctx context.Context) (Plan, error) {
	return RetryFailed(ctx, plan)
}

// Apply verifies plan completeness and the source fingerprint, then writes a
// new workbook. In partial mode every row with an issue is left wholly intact;
// formulas are never replaced under any option.
func Apply(ctx context.Context, plan Plan, request ApplyRequest) (Report, error) {
	if plan.state == nil {
		return Report{}, &rtasales.InputError{Field: "Plan", Message: "was not produced by Analyze"}
	}
	state := plan.state
	state.mu.Lock()
	defer state.mu.Unlock()
	state.rebuildReportLocked()
	report := state.report
	if state.executing {
		return report, &PlanBusyError{}
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	if !state.complete {
		return report, &IncompletePlanError{}
	}
	outputPath, err := validateDistinctOutput(state.inputPath, request.OutputPath)
	if err != nil {
		return report, err
	}
	actual, err := fingerprintFileContext(ctx, state.inputPath)
	if err != nil {
		return report, fmt.Errorf("fingerprint source workbook: %w", err)
	}
	if !sameFingerprint(state.fingerprint, actual) {
		return report, &SourceChangedError{Expected: state.fingerprint, Actual: actual}
	}
	if len(report.Issues) > 0 && !request.AllowPartial {
		return report, &ValidationError{IssueCount: len(report.Issues)}
	}

	book, err := excelize.OpenFile(state.inputPath, excelize.Options{RawCellValue: true})
	if err != nil {
		return report, fmt.Errorf("open source workbook: %w", err)
	}
	defer func() { _ = book.Close() }()
	for _, row := range state.rows {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		if row.status != RowStatusReady {
			continue
		}
		for _, update := range row.updates {
			if err := book.SetCellValue(state.sheet, update.cell, update.value); err != nil {
				return report, fmt.Errorf("write worksheet cell: %w", err)
			}
		}
	}
	if request.ForceRecalculate {
		if err := forceWorkbookRecalculation(book); err != nil {
			return report, err
		}
	}
	if err := publishWorkbook(ctx, book, state.inputPath, outputPath, state.fingerprint); err != nil {
		return report, err
	}
	report.WroteWorkbook = true
	return report, nil
}

func forceWorkbookRecalculation(book *excelize.File) error {
	auto := "auto"
	yes := true
	if err := book.SetCalcProps(&excelize.CalcPropsOptions{
		CalcMode: &auto, FullCalcOnLoad: &yes, ForceFullCalc: &yes, CalcOnSave: &yes,
	}); err != nil {
		return fmt.Errorf("set workbook calculation mode: %w", err)
	}
	return nil
}

// publishWorkbook serializes into an exclusive temporary file and only then
// atomically replaces the destination. SaveAs opens its destination with
// truncation, which could otherwise destroy the source if the destination were
// swapped to a hardlink after validation.
func publishWorkbook(ctx context.Context, book *excelize.File, inputPath, outputPath string, expected SourceFingerprint) error {
	parent := filepath.Dir(outputPath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	temporary, err := os.CreateTemp(parent, ".rta-xlsx-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary output workbook: %w", err)
	}
	temporaryPath := temporary.Name()
	temporaryOpen := true
	published := false
	defer func() {
		if temporaryOpen {
			_ = temporary.Close()
		}
		if !published {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := book.Write(contextWriter{ctx: ctx, writer: temporary}); err != nil {
		return fmt.Errorf("serialize output workbook: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary output workbook: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary output workbook: %w", err)
	}
	temporaryOpen = false
	if err := ctx.Err(); err != nil {
		return err
	}
	// Catch a source change that raced with opening or serializing the in-memory
	// copy. The destination identity is also checked again immediately before
	// atomic publication.
	actual, err := fingerprintFileContext(ctx, inputPath)
	if err != nil {
		return fmt.Errorf("fingerprint source workbook: %w", err)
	}
	if !sameFingerprint(expected, actual) {
		return &SourceChangedError{Expected: expected, Actual: actual}
	}
	outputPath, err = validateDistinctOutput(inputPath, outputPath)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := replaceWorkbookFile(temporaryPath, outputPath); err != nil {
		return fmt.Errorf("publish output workbook: %w", err)
	}
	published = true
	return nil
}

func prepareBatchPlan(ctx context.Context, provider SalesProvider, request BatchRequest) (*batchPlanState, error) {
	if nilSalesProvider(provider) {
		return nil, &rtasales.InputError{Field: "provider", Message: "is required"}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	request.InputPath = strings.TrimSpace(request.InputPath)
	if request.InputPath == "" {
		return nil, &rtasales.InputError{Field: "InputPath", Message: "is required"}
	}
	absoluteInput, err := filepath.Abs(request.InputPath)
	if err != nil {
		return nil, fmt.Errorf("resolve input path: %w", err)
	}
	request.InputPath = filepath.Clean(absoluteInput)
	if request.From.IsZero() {
		return nil, &rtasales.InputError{Field: "From", Message: "is required"}
	}
	if request.To.IsZero() {
		return nil, &rtasales.InputError{Field: "To", Message: "is required"}
	}
	if calendarKey(request.To) < calendarKey(request.From) {
		return nil, &rtasales.InputError{Field: "To", Message: "must not precede From"}
	}
	if request.OnlyRow < 0 || request.OnlyRow == 1 {
		return nil, &rtasales.InputError{Field: "OnlyRow", Message: "must be zero or a data row greater than one"}
	}
	if request.Mapper == nil {
		request.Mapper = IdentityStoreMap{}
	}
	allowed, err := normalizeAllowedStoreIDs(request.AllowedBusinessStoreIDs)
	if err != nil {
		return nil, err
	}
	if request.SheetName == "" {
		request.SheetName = DefaultSheetName
	}
	if request.MaxJobs == 0 {
		request.MaxJobs = DefaultMaxJobs
	}
	if request.MaxJobs < 0 {
		return nil, &rtasales.InputError{Field: "MaxJobs", Message: "must be positive"}
	}
	if request.Concurrency == 0 {
		request.Concurrency = DefaultConcurrency
	}
	if request.Concurrency < 1 || request.Concurrency > MaximumConcurrency {
		return nil, &rtasales.InputError{Field: "Concurrency", Message: fmt.Sprintf("must be between 1 and %d", MaximumConcurrency)}
	}
	if request.Progress != nil && request.ProgressCallback != nil {
		return nil, &rtasales.InputError{Field: "Progress", Message: "set only one progress callback"}
	}
	progress := request.Progress
	if progress == nil {
		progress = request.ProgressCallback
	}
	backoff := request.Backoff
	if backoff == nil {
		backoff = waitForRetry
	}
	request.AllowedBusinessStoreIDs = append([]string(nil), request.AllowedBusinessStoreIDs...)
	request.Progress = progress
	request.ProgressCallback = nil
	request.Backoff = backoff
	state := &batchPlanState{
		resumeProvider: provider, resumeRequest: request,
		inputPath: request.InputPath, sheet: request.SheetName,
		from: request.From, to: request.To,
		overwrite: request.Overwrite, concurrency: request.Concurrency,
		progress: progress, backoff: backoff, extraIssues: make(map[string][]int),
		report: Report{
			Date: singleDateText(request.From, request.To),
			From: request.From.Format("2006-01-02"), To: request.To.Format("2006-01-02"),
			Sheet: request.SheetName,
		},
	}
	fingerprint, err := fingerprintFileContext(ctx, request.InputPath)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			state.rebuildReportLocked()
			return state, contextErr
		}
		return nil, fmt.Errorf("fingerprint source workbook: %w", err)
	}
	state.fingerprint = fingerprint
	book, err := excelize.OpenFile(request.InputPath, excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, fmt.Errorf("open workbook: %w", err)
	}
	defer func() { _ = book.Close() }()
	if index, inspectErr := book.GetSheetIndex(request.SheetName); inspectErr != nil || index == -1 {
		if inspectErr != nil {
			return nil, fmt.Errorf("inspect worksheet: %w", inspectErr)
		}
		return nil, &rtasales.InputError{Field: "SheetName", Message: "worksheet does not exist"}
	}
	maxRow, err := usedRangeLastRow(book, request.SheetName)
	if err != nil {
		return nil, err
	}
	props, err := book.GetWorkbookProps()
	if err != nil {
		return nil, fmt.Errorf("read workbook properties: %w", err)
	}
	use1904 := props.Date1904 != nil && *props.Date1904

	type jobKey struct {
		date    string
		storeID string
	}
	jobsByKey := make(map[jobKey]int)
	skippedAuthorizedRows := make([]int, 0)
	fromKey, toKey := calendarKey(request.From), calendarKey(request.To)
	for rowNumber := 2; rowNumber <= maxRow; rowNumber++ {
		if err := ctx.Err(); err != nil {
			state.rebuildReportLocked()
			return state, err
		}
		if request.OnlyRow > 0 && rowNumber != request.OnlyRow {
			continue
		}
		storeID, err := rawCell(book, request.SheetName, "C", rowNumber)
		if err != nil {
			return nil, err
		}
		dateRaw, err := rawCell(book, request.SheetName, "F", rowNumber)
		if err != nil {
			return nil, err
		}
		label, err := rawCell(book, request.SheetName, "E", rowNumber)
		if err != nil {
			return nil, err
		}
		if strings.EqualFold(strings.TrimSpace(label), "Total") || strings.TrimSpace(storeID) == "" || strings.TrimSpace(dateRaw) == "" {
			state.report.SkippedDataRows++
			continue
		}
		rowDate, parseErr := parseWorkbookDate(book, request.SheetName, rowNumber, dateRaw, use1904)
		if parseErr != nil {
			state.rows = append(state.rows, batchRow{
				number: rowNumber, workbookStoreID: strings.TrimSpace(storeID),
				status: RowStatusIssue, static: []string{"invalid_date"}, jobIndex: -1,
			})
			continue
		}
		rowDate = calendarDate(rowDate)
		dateKey := calendarKey(rowDate)
		if dateKey < fromKey || dateKey > toKey {
			continue
		}
		state.report.MatchedRows++
		current, err := readCurrentValues(book, request.SheetName, rowNumber)
		if err != nil {
			return nil, err
		}
		row := batchRow{
			number: rowNumber, date: rowDate.Format("2006-01-02"),
			workbookStoreID: strings.TrimSpace(storeID), current: current,
			status: RowStatusPending, jobIndex: -1,
		}
		businessStoreID, ok := request.Mapper.ResolveStore(strings.TrimSpace(storeID))
		businessStoreID = strings.TrimSpace(businessStoreID)
		if !ok || businessStoreID == "" {
			row.status = RowStatusIssue
			row.static = []string{"missing_mapping"}
			state.rows = append(state.rows, row)
			continue
		}
		if len(allowed) > 0 {
			if _, ok := allowed[businessStoreID]; !ok {
				row.status = RowStatusSkipped
				state.rows = append(state.rows, row)
				state.report.SkippedStoreRows++
				skippedAuthorizedRows = append(skippedAuthorizedRows, rowNumber)
				continue
			}
		}
		state.report.SelectedRows++
		jobProvider, profile, lane, ok := resolveBatchProvider(provider, businessStoreID)
		if !ok {
			row.status = RowStatusIssue
			row.static = []string{"store_not_authorized"}
			state.rows = append(state.rows, row)
			continue
		}
		row.profile = profile
		key := jobKey{date: row.date, storeID: businessStoreID}
		jobIndex, exists := jobsByKey[key]
		rowIndex := len(state.rows)
		if !exists {
			jobIndex = len(state.jobs)
			jobsByKey[key] = jobIndex
			state.jobs = append(state.jobs, batchJob{
				storeID: businessStoreID, date: rowDate, dateText: row.date,
				profile: profile, lane: lane, provider: jobProvider,
			})
		}
		row.jobIndex = jobIndex
		state.rows = append(state.rows, row)
		state.jobs[jobIndex].rowIndexes = append(state.jobs[jobIndex].rowIndexes, rowIndex)
	}
	if len(allowed) > 0 && state.report.SelectedRows == 0 && len(skippedAuthorizedRows) > 0 {
		state.extraIssues["no_authorized_store_match"] = append([]int(nil), skippedAuthorizedRows...)
	}
	state.report.UniqueQueries = len(state.jobs)
	if len(state.jobs) > request.MaxJobs {
		return nil, &rtasales.InputError{Field: "MaxJobs", Message: fmt.Sprintf("matched %d date/store jobs, limit is %d", len(state.jobs), request.MaxJobs)}
	}
	state.scanComplete = true
	state.complete = len(state.jobs) == 0
	state.rebuildReportLocked()
	return state, nil
}

func (state *batchPlanState) executeJobsLocked(ctx context.Context, indexes []int) error {
	if len(indexes) == 0 {
		state.complete = state.allJobsTerminalLocked()
		state.rebuildReportLocked()
		return ctx.Err()
	}
	if state.executing {
		return &PlanBusyError{}
	}
	state.executing = true
	defer func() { state.executing = false }()
	// RTA applies throttling to the authenticated session, not to an individual
	// store. Keep jobs sharing one cookie in a serial lane, including their
	// retry waits, while still allowing independent accounts to run together.
	jobsByLane := make(map[string][]int)
	laneOrder := make([]string, 0)
	for _, index := range indexes {
		lane := state.jobs[index].lane
		if _, exists := jobsByLane[lane]; !exists {
			laneOrder = append(laneOrder, lane)
		}
		jobsByLane[lane] = append(jobsByLane[lane], index)
	}
	workerCount := state.concurrency
	if workerCount > len(laneOrder) {
		workerCount = len(laneOrder)
	}
	work := make(chan []int)
	results := make(chan batchJobResult, workerCount)
	var workers sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for lane := range work {
				for _, index := range lane {
					if ctx.Err() != nil {
						return
					}
					result := executeBatchJob(ctx, state.jobs[index], state.backoff)
					result.index = index
					results <- result
					if result.cancelled {
						return
					}
				}
			}
		}()
	}
	go func() {
		defer close(work)
		for _, lane := range laneOrder {
			select {
			case work <- jobsByLane[lane]:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()
	var firstExecutionError error
	for result := range results {
		job := &state.jobs[result.index]
		job.attempts += result.attempts
		if result.cancelled {
			job.status = batchJobPending
			if firstExecutionError == nil && result.err != nil {
				firstExecutionError = result.err
			}
			continue
		}
		job.status = result.status
		job.issueCode = result.issueCode
		for _, rowIndex := range job.rowIndexes {
			row := &state.rows[rowIndex]
			row.profile = job.profile
			row.queryIssue = result.issueCode
			row.cellIssues = nil
			row.updates = nil
			row.unchanged = 0
			if result.status == batchJobSucceeded {
				state.populateSuccessfulRowLocked(row, result.result)
			} else {
				row.status = RowStatusIssue
			}
		}
		state.rebuildReportLocked()
		if state.progress != nil {
			status := "success"
			if result.status != batchJobSucceeded {
				status = "issue"
			}
			state.callProgressLocked(ProgressEvent{
				Stage: "analyze", CompletedJobs: state.report.CompletedJobs, TotalJobs: len(state.jobs),
				Attempt: result.attempts, Date: job.dateText,
				Profile: job.profile, StoreID: job.storeID, Status: status,
			})
		}
	}
	state.complete = state.allJobsTerminalLocked()
	state.rebuildReportLocked()
	if err := ctx.Err(); err != nil && !state.complete {
		return err
	}
	if firstExecutionError != nil {
		return firstExecutionError
	}
	return nil
}

// callProgressLocked invokes application code without holding the plan mutex.
// The deferred lock also restores the caller's invariant if the callback
// panics. executing keeps re-entrant mutating operations fail-closed.
func (state *batchPlanState) callProgressLocked(event ProgressEvent) {
	callback := state.progress
	state.mu.Unlock()
	defer state.mu.Lock()
	callback(event)
}

func executeBatchJob(ctx context.Context, job batchJob, backoff BackoffFunc) batchJobResult {
	for attempt := 1; attempt <= len(defaultRetryDelays)+1; attempt++ {
		if err := ctx.Err(); err != nil {
			return batchJobResult{status: batchJobPending, attempts: attempt - 1, cancelled: true, err: err}
		}
		result, err := job.provider.Sales(ctx, rtasales.SalesQuery{
			BusinessStoreID: job.storeID,
			StartDate:       job.date,
			EndDate:         job.date,
		})
		if err == nil {
			if code := validateSalesResult(result); code != "" {
				return batchJobResult{status: batchJobTerminalIssue, issueCode: code, attempts: attempt}
			}
			return batchJobResult{status: batchJobSucceeded, result: result, attempts: attempt}
		}
		if ctx.Err() != nil {
			return batchJobResult{status: batchJobPending, attempts: attempt, cancelled: true, err: ctx.Err()}
		}
		retryable := isTemporaryQueryError(err)
		if !retryable {
			return batchJobResult{status: batchJobTerminalIssue, issueCode: classifyQueryError(err), attempts: attempt}
		}
		if attempt > len(defaultRetryDelays) {
			return batchJobResult{status: batchJobRetryableFailure, issueCode: classifyQueryError(err), attempts: attempt}
		}
		if err := backoff(ctx, defaultRetryDelays[attempt-1]); err != nil {
			return batchJobResult{status: batchJobPending, attempts: attempt, cancelled: true, err: err}
		}
	}
	panic("unreachable")
}

func validateSalesResult(result *rtasales.SalesResult) string {
	if result == nil || len(result.Items) == 0 {
		return "no_data"
	}
	sales := workbookSalesAmount(result)
	if math.IsNaN(sales) || math.IsInf(sales, 0) {
		return "invalid_sales_total"
	}
	if result.TotalTransactionCount == nil || math.IsNaN(*result.TotalTransactionCount) || math.IsInf(*result.TotalTransactionCount, 0) {
		return "transaction_total_unavailable"
	}
	count := *result.TotalTransactionCount
	if count < 0 || math.Abs(count-math.Round(count)) > 1e-9 {
		return "invalid_transaction_total"
	}
	return ""
}

func (state *batchPlanState) populateSuccessfulRowLocked(row *batchRow, result *rtasales.SalesResult) {
	sales := workbookSalesAmount(result)
	transactions := math.Round(*result.TotalTransactionCount)
	row.proposed = ProposedValues{DailySales: floatPointer(sales), TransactionCount: floatPointer(transactions)}
	row.status = RowStatusReady
	for _, target := range []struct {
		cell     string
		current  string
		formula  string
		proposed float64
	}{
		{cell: "L" + strconv.Itoa(row.number), current: row.current.DailySales, formula: row.current.DailySalesFormula, proposed: sales},
		{cell: "AB" + strconv.Itoa(row.number), current: row.current.TransactionCount, formula: row.current.TransactionCountFormula, proposed: transactions},
	} {
		if strings.TrimSpace(target.formula) != "" {
			row.cellIssues = append(row.cellIssues, "target_contains_formula")
			continue
		}
		current := strings.TrimSpace(target.current)
		if current != "" {
			if number, err := strconv.ParseFloat(current, 64); err == nil && nearlyEqual(number, target.proposed) {
				row.unchanged++
				continue
			}
			if !state.overwrite {
				row.cellIssues = append(row.cellIssues, "existing_value_differs")
				continue
			}
		}
		row.updates = append(row.updates, cellUpdate{cell: target.cell, value: target.proposed})
	}
	if len(row.cellIssues) > 0 {
		row.status = RowStatusIssue
		return
	}
	if len(row.updates) == 0 {
		row.status = RowStatusUnchanged
	}
}

func workbookSalesAmount(result *rtasales.SalesResult) float64 {
	if result.TrendGrossSaleAmount != nil {
		return *result.TrendGrossSaleAmount
	}
	// Preserve compatibility with custom SalesProvider implementations that
	// predate the Trend View aggregate field.
	return result.TotalAmount
}

func (state *batchPlanState) rebuildReportLocked() {
	report := state.report
	report.StagedCells = 0
	report.UnchangedCells = 0
	report.CompletedJobs = 0
	report.FailedJobs = 0
	issues := make(map[string][]int)
	for code, rows := range state.extraIssues {
		issues[code] = append(issues[code], rows...)
	}
	for _, row := range state.rows {
		for _, code := range row.static {
			issues[code] = append(issues[code], row.number)
		}
		if row.queryIssue != "" {
			issues[row.queryIssue] = append(issues[row.queryIssue], row.number)
		}
		for _, code := range row.cellIssues {
			issues[code] = append(issues[code], row.number)
		}
		if row.status == RowStatusReady {
			report.StagedCells += len(row.updates)
		}
		report.UnchangedCells += row.unchanged
	}
	for _, job := range state.jobs {
		switch job.status {
		case batchJobSucceeded, batchJobTerminalIssue, batchJobRetryableFailure:
			report.CompletedJobs++
		}
		if job.status == batchJobRetryableFailure {
			report.FailedJobs++
		}
	}
	report.Issues = sortedIssues(issues)
	report.Complete = state.complete
	state.report = report
}

func (state *batchPlanState) allJobsTerminalLocked() bool {
	for _, job := range state.jobs {
		if job.status == batchJobPending {
			return false
		}
	}
	return true
}

func (state *batchPlanState) snapshotLocked() Plan {
	state.rebuildReportLocked()
	rows := make([]RowPlan, len(state.rows))
	for index, row := range state.rows {
		issues := append([]string(nil), row.static...)
		if row.queryIssue != "" {
			issues = append(issues, row.queryIssue)
		}
		issues = append(issues, row.cellIssues...)
		sort.Strings(issues)
		issues = uniqueStrings(issues)
		rows[index] = RowPlan{
			Row: row.number, Date: row.date, WorkbookStoreID: row.workbookStoreID,
			Current: row.current, Proposed: copyProposed(row.proposed), Status: row.status,
			Profile: row.profile, Issues: issues,
		}
	}
	return Plan{
		InputPath: state.inputPath, Sheet: state.sheet, From: state.from, To: state.to,
		Source: state.fingerprint, Rows: rows,
		Issues: appendIssues(state.report.Issues), Complete: state.complete,
		Report: state.report, state: state,
	}
}

func readCurrentValues(book *excelize.File, sheet string, row int) (CurrentValues, error) {
	var values CurrentValues
	var err error
	values.DailySales, err = rawCell(book, sheet, "L", row)
	if err != nil {
		return values, err
	}
	values.TransactionCount, err = rawCell(book, sheet, "AB", row)
	if err != nil {
		return values, err
	}
	values.DailySalesFormula, err = book.GetCellFormula(sheet, "L"+strconv.Itoa(row))
	if err != nil {
		return values, fmt.Errorf("inspect target formula: %w", err)
	}
	values.TransactionCountFormula, err = book.GetCellFormula(sheet, "AB"+strconv.Itoa(row))
	if err != nil {
		return values, fmt.Errorf("inspect target formula: %w", err)
	}
	return values, nil
}

func resolveBatchProvider(provider SalesProvider, storeID string) (SalesProvider, string, string, bool) {
	if router, ok := provider.(interface {
		ProviderForStore(string) (ProviderRoute, bool)
	}); ok {
		route, found := router.ProviderForStore(storeID)
		if !found || nilSalesProvider(route.Provider) {
			return nil, "", "", false
		}
		lane := strings.TrimSpace(route.Lane)
		if lane == "" {
			lane = providerLane(route.Provider)
		}
		return route.Provider, strings.TrimSpace(route.Profile), lane, true
	}
	return provider, "", providerLane(provider), true
}

func isTemporaryQueryError(err error) bool {
	return rtasales.IsRetryable(err)
}

func isTransientProtocolError(err error) bool {
	var protocolError *rtasales.ProtocolError
	return errors.As(err, &protocolError) && protocolError.Retryable()
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func fingerprintFileContext(ctx context.Context, path string) (SourceFingerprint, error) {
	if err := ctx.Err(); err != nil {
		return SourceFingerprint{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return SourceFingerprint{}, err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, contextReader{ctx: ctx, reader: file}); err != nil {
		return SourceFingerprint{}, err
	}
	info, err := file.Stat()
	if err != nil {
		return SourceFingerprint{}, err
	}
	return SourceFingerprint{
		SHA256: hex.EncodeToString(hash.Sum(nil)), Size: info.Size(), ModTime: info.ModTime(),
	}, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}

type contextWriter struct {
	ctx    context.Context
	writer io.Writer
}

func (writer contextWriter) Write(buffer []byte) (int, error) {
	if err := writer.ctx.Err(); err != nil {
		return 0, err
	}
	return writer.writer.Write(buffer)
}

func sameFingerprint(left, right SourceFingerprint) bool {
	return left.SHA256 == right.SHA256 && left.Size == right.Size && left.ModTime.Equal(right.ModTime)
}

func validateDistinctOutput(inputPath, outputPath string) (string, error) {
	outputPath = strings.TrimSpace(outputPath)
	if outputPath == "" {
		return "", &rtasales.InputError{Field: "OutputPath", Message: "is required"}
	}
	inputAbs, err := filepath.Abs(inputPath)
	if err != nil {
		return "", fmt.Errorf("resolve input path: %w", err)
	}
	outputAbs, err := filepath.Abs(outputPath)
	if err != nil {
		return "", fmt.Errorf("resolve output path: %w", err)
	}
	if strings.EqualFold(filepath.Clean(inputAbs), filepath.Clean(outputAbs)) {
		return "", &rtasales.InputError{Field: "OutputPath", Message: "must differ from InputPath"}
	}
	inputInfo, inputErr := os.Stat(inputAbs)
	outputInfo, outputErr := os.Stat(outputAbs)
	if inputErr == nil && outputErr == nil && os.SameFile(inputInfo, outputInfo) {
		return "", &rtasales.InputError{Field: "OutputPath", Message: "must differ from InputPath"}
	}
	return filepath.Clean(outputAbs), nil
}

// ValidateOutputPath performs the same in-place and existing-hardlink checks
// used by Apply. Commands should call it before making network requests while
// retaining Apply as the final safety boundary.
func ValidateOutputPath(inputPath, outputPath string) error {
	_, err := validateDistinctOutput(inputPath, outputPath)
	return err
}

func calendarDate(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}

func calendarKey(value time.Time) string { return value.Format("20060102") }

func singleDateText(from, to time.Time) string {
	if calendarKey(from) == calendarKey(to) {
		return from.Format("2006-01-02")
	}
	return ""
}

func allJobIndexes(jobs []batchJob) []int {
	indexes := make([]int, len(jobs))
	for index := range jobs {
		indexes[index] = index
	}
	return indexes
}

func floatPointer(value float64) *float64 {
	copyOfValue := value
	return &copyOfValue
}

func copyProposed(values ProposedValues) ProposedValues {
	copyOfValues := ProposedValues{}
	if values.DailySales != nil {
		copyOfValues.DailySales = floatPointer(*values.DailySales)
	}
	if values.TransactionCount != nil {
		copyOfValues.TransactionCount = floatPointer(*values.TransactionCount)
	}
	return copyOfValues
}

func appendIssues(input []Issue) []Issue {
	result := make([]Issue, len(input))
	for index, issue := range input {
		result[index] = Issue{Code: issue.Code, Rows: append([]int(nil), issue.Rows...)}
	}
	return result
}

func uniqueStrings(values []string) []string {
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
