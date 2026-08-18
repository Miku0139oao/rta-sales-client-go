package xlsxfill

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	rtasales "github.com/Miku0139oao/rta-sales-client-go/rtasales"
	"github.com/xuri/excelize/v2"
)

type batchTestProvider struct {
	mu      sync.Mutex
	calls   []rtasales.SalesQuery
	handler func(context.Context, rtasales.SalesQuery, int) (*rtasales.SalesResult, error)
}

func (provider *batchTestProvider) Sales(ctx context.Context, query rtasales.SalesQuery) (*rtasales.SalesResult, error) {
	provider.mu.Lock()
	provider.calls = append(provider.calls, query)
	call := len(provider.calls)
	provider.mu.Unlock()
	if provider.handler != nil {
		return provider.handler(ctx, query, call)
	}
	return batchResult(100, 10), nil
}

func (provider *batchTestProvider) Calls() []rtasales.SalesQuery {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return append([]rtasales.SalesQuery(nil), provider.calls...)
}

func batchResult(amount, transactions float64) *rtasales.SalesResult {
	return &rtasales.SalesResult{
		TotalAmount: amount, TotalTransactionCount: floatPointer(transactions),
		Items: []rtasales.SaleItem{{Matnr: "item"}},
	}
}

type batchWorkbookRow struct {
	store string
	label string
	date  any
	l     any
	ab    any
}

func createBatchWorkbook(t *testing.T, path string, rows []batchWorkbookRow) {
	t.Helper()
	book := excelize.NewFile()
	defer func() { _ = book.Close() }()
	if err := book.SetSheetName("Sheet1", DefaultSheetName); err != nil {
		t.Fatal(err)
	}
	for cell, value := range map[string]any{
		"C1": "Store ID", "E1": "Store ABR", "F1": "Date", "L1": "Daily Sales", "AB1": "Transactions",
	} {
		if err := book.SetCellValue(DefaultSheetName, cell, value); err != nil {
			t.Fatal(err)
		}
	}
	dateStyle, err := book.NewStyle(&excelize.Style{NumFmt: 14})
	if err != nil {
		t.Fatal(err)
	}
	for index, row := range rows {
		number := index + 2
		for column, value := range map[string]any{"C": row.store, "E": row.label, "F": row.date, "L": row.l, "AB": row.ab} {
			if value == nil {
				continue
			}
			if err := book.SetCellValue(DefaultSheetName, column+strconv.Itoa(number), value); err != nil {
				t.Fatal(err)
			}
		}
		if _, ok := row.date.(time.Time); ok {
			cell := "F" + strconv.Itoa(number)
			if err := book.SetCellStyle(DefaultSheetName, cell, cell, dateStyle); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := book.SaveAs(path); err != nil {
		t.Fatal(err)
	}
}

func TestAnalyzeMultiDayDeduplicatesAndUsesDailyQueries(t *testing.T) {
	input := filepath.Join(t.TempDir(), "cross-month.xlsx")
	jul31 := time.Date(2026, time.July, 31, 0, 0, 0, 0, time.Local)
	aug1 := jul31.AddDate(0, 0, 1)
	aug2 := jul31.AddDate(0, 0, 2)
	aug3 := jul31.AddDate(0, 0, 3)
	createBatchWorkbook(t, input, []batchWorkbookRow{
		{store: "A", label: "A", date: jul31},
		{store: "A", label: "A duplicate", date: jul31},
		{store: "A", label: "A", date: aug1},
		{store: "B", label: "B", date: aug1},
		{store: "A", label: "outside", date: aug3},
		{store: "", label: "Total", date: aug1, l: 999, ab: 99},
	})
	provider := &batchTestProvider{handler: func(_ context.Context, query rtasales.SalesQuery, _ int) (*rtasales.SalesResult, error) {
		amount := 100.0
		if query.BusinessStoreID == "B" {
			amount = 200
		}
		if query.StartDate.Day() == 1 {
			amount += 1
		}
		return batchResult(amount, 10), nil
	}}
	plan, err := Analyze(context.Background(), provider, BatchRequest{
		InputPath: input, From: jul31, To: aug2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Complete || !plan.IsComplete() || plan.Report.MatchedRows != 4 || plan.Report.UniqueQueries != 3 || plan.Report.StagedCells != 8 {
		t.Fatalf("unexpected plan report: %+v", plan.Report)
	}
	if len(plan.Rows) != 4 || plan.Rows[0].WorkbookStoreID != "A" || plan.Rows[0].Date != "2026-07-31" {
		t.Fatalf("unexpected rows: %+v", plan.Rows)
	}
	if plan.Rows[0].Proposed.DailySales == nil || *plan.Rows[0].Proposed.DailySales != *plan.Rows[1].Proposed.DailySales {
		t.Fatalf("duplicate rows did not reuse a proposal: %+v", plan.Rows[:2])
	}
	calls := provider.Calls()
	if len(calls) != 3 {
		t.Fatalf("calls=%d, want 3", len(calls))
	}
	seen := make(map[string]int)
	for _, call := range calls {
		if !sameCalendarDate(call.StartDate, call.EndDate) {
			t.Fatalf("query was not daily: %+v", call)
		}
		seen[call.BusinessStoreID+"/"+call.StartDate.Format("2006-01-02")]++
	}
	if len(seen) != 3 {
		t.Fatalf("queries were not unique by date/store: %+v", seen)
	}
}

type laneTestProvider struct {
	name         string
	started      chan<- string
	release      <-chan struct{}
	active       atomic.Int32
	maxActive    atomic.Int32
	globalActive *atomic.Int32
	globalMax    *atomic.Int32
}

func (provider *laneTestProvider) Sales(ctx context.Context, _ rtasales.SalesQuery) (*rtasales.SalesResult, error) {
	active := provider.active.Add(1)
	updateAtomicMax(&provider.maxActive, active)
	global := provider.globalActive.Add(1)
	updateAtomicMax(provider.globalMax, global)
	defer func() {
		provider.active.Add(-1)
		provider.globalActive.Add(-1)
	}()
	select {
	case provider.started <- provider.name:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case <-provider.release:
		return batchResult(1, 1), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func updateAtomicMax(target *atomic.Int32, value int32) {
	for current := target.Load(); value > current && !target.CompareAndSwap(current, value); current = target.Load() {
	}
}

func TestAnalyzeRunsJobsConcurrentlyForOneAccount(t *testing.T) {
	input := filepath.Join(t.TempDir(), "accounts.xlsx")
	date := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.Local)
	createBatchWorkbook(t, input, []batchWorkbookRow{
		{store: "A1", label: "A1", date: date},
		{store: "A2", label: "A2", date: date.AddDate(0, 0, 1)},
		{store: "B1", label: "B1", date: date},
		{store: "B1", label: "B1", date: date.AddDate(0, 0, 1)},
	})
	started := make(chan string, 4)
	release := make(chan struct{})
	var globalActive, globalMax atomic.Int32
	accountA := &laneTestProvider{name: "A", started: started, release: release, globalActive: &globalActive, globalMax: &globalMax}
	accountB := &laneTestProvider{name: "B", started: started, release: release, globalActive: &globalActive, globalMax: &globalMax}
	router, err := NewProfiledProviderRouter(map[string]ProviderRoute{
		"A1": {Provider: accountA, Profile: "Profile A"},
		"A2": {Provider: accountA, Profile: "Profile A"},
		"B1": {Provider: accountB, Profile: "Profile B"},
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var plan Plan
	var analyzeErr error
	go func() {
		plan, analyzeErr = Analyze(context.Background(), router, BatchRequest{
			InputPath: input, From: date, To: date.AddDate(0, 0, 1), Concurrency: 2,
		})
		close(done)
	}()
	first := waitStarted(t, started)
	second := waitStarted(t, started)
	if first != "A" || second != "A" {
		t.Fatalf("first account did not run two jobs concurrently: %q and %q", first, second)
	}
	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("analysis did not finish")
	}
	if analyzeErr != nil {
		t.Fatal(analyzeErr)
	}
	if accountA.maxActive.Load() != 2 || globalMax.Load() != 2 {
		t.Fatalf("max active A=%d B=%d global=%d", accountA.maxActive.Load(), accountB.maxActive.Load(), globalMax.Load())
	}
	profiles := make(map[string]bool)
	for _, row := range plan.Rows {
		profiles[row.Profile] = true
	}
	if !profiles["Profile A"] || !profiles["Profile B"] {
		t.Fatalf("route profiles missing: %+v", plan.Rows)
	}
}

func waitStarted(t *testing.T, started <-chan string) string {
	t.Helper()
	select {
	case value := <-started:
		return value
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for concurrent job")
		return ""
	}
}

func TestAnalyzeRetriesOnlyTemporaryFailuresAndRetryFailedResumes(t *testing.T) {
	input := filepath.Join(t.TempDir(), "retry.xlsx")
	date := time.Date(2026, time.August, 2, 0, 0, 0, 0, time.Local)
	createBatchWorkbook(t, input, []batchWorkbookRow{{store: "A", label: "A", date: date}})
	delays := make([]time.Duration, 0)
	provider := &batchTestProvider{handler: func(_ context.Context, _ rtasales.SalesQuery, call int) (*rtasales.SalesResult, error) {
		if call <= 2 {
			return nil, &rtasales.UpstreamError{Operation: "sales", StatusCode: 503}
		}
		return batchResult(30, 3), nil
	}}
	plan, err := Analyze(context.Background(), provider, BatchRequest{
		InputPath: input, From: date, To: date,
		Backoff: func(_ context.Context, delay time.Duration) error {
			delays = append(delays, delay)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.Calls()) != 3 || len(delays) != 2 || delays[0] != time.Second || delays[1] != 3*time.Second || len(plan.Issues) != 0 {
		t.Fatalf("calls=%d delays=%v plan=%+v", len(provider.Calls()), delays, plan.Report)
	}
	stopBackoff := errors.New("stop retry wait")
	stopped := &batchTestProvider{handler: func(context.Context, rtasales.SalesQuery, int) (*rtasales.SalesResult, error) {
		return nil, &rtasales.UpstreamError{Operation: "sales", StatusCode: 503}
	}}
	stoppedPlan, err := Analyze(context.Background(), stopped, BatchRequest{
		InputPath: input, From: date, To: date,
		Backoff: func(context.Context, time.Duration) error { return stopBackoff },
	})
	if !errors.Is(err, stopBackoff) || stoppedPlan.Complete || len(stopped.Calls()) != 1 {
		t.Fatalf("backoff error=%v complete=%t calls=%d", err, stoppedPlan.Complete, len(stopped.Calls()))
	}

	noData := &batchTestProvider{handler: func(context.Context, rtasales.SalesQuery, int) (*rtasales.SalesResult, error) {
		return &rtasales.SalesResult{}, nil
	}}
	noDataPlan, err := Analyze(context.Background(), noData, BatchRequest{
		InputPath: input, From: date, To: date,
		Backoff: func(context.Context, time.Duration) error { t.Fatal("no-data must not retry"); return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(noData.Calls()) != 1 || len(noDataPlan.Issues) != 1 || noDataPlan.Issues[0].Code != "no_data" {
		t.Fatalf("unexpected no-data plan: %+v calls=%d", noDataPlan.Report, len(noData.Calls()))
	}
	for name, terminalError := range map[string]error{
		"permission": &rtasales.AuthError{Code: "403", Message: "denied"},
		"format":     &rtasales.ProtocolError{Operation: "sales", Message: "bad data"},
		"store":      &rtasales.StoreNotFoundError{BusinessStoreID: "A"},
		"http403":    &rtasales.UpstreamError{Operation: "sales", StatusCode: 403},
	} {
		t.Run("does not retry "+name, func(t *testing.T) {
			terminal := &batchTestProvider{handler: func(context.Context, rtasales.SalesQuery, int) (*rtasales.SalesResult, error) {
				return nil, terminalError
			}}
			var waits atomic.Int32
			terminalPlan, err := Analyze(context.Background(), terminal, BatchRequest{
				InputPath: input, From: date, To: date,
				Backoff: func(context.Context, time.Duration) error { waits.Add(1); return nil },
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(terminal.Calls()) != 1 || waits.Load() != 0 || len(terminalPlan.Issues) == 0 {
				t.Fatalf("calls=%d waits=%d plan=%+v", len(terminal.Calls()), waits.Load(), terminalPlan.Report)
			}
		})
	}

	var recovered atomic.Bool
	failing := &batchTestProvider{handler: func(_ context.Context, _ rtasales.SalesQuery, _ int) (*rtasales.SalesResult, error) {
		if !recovered.Load() {
			return nil, &rtasales.UpstreamError{Operation: "sales", StatusCode: 503}
		}
		return batchResult(40, 4), nil
	}}
	failedPlan, err := Analyze(context.Background(), failing, BatchRequest{
		InputPath: input, From: date, To: date,
		Backoff: func(context.Context, time.Duration) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if failedPlan.Report.FailedJobs != 1 || len(failing.Calls()) != 3 {
		t.Fatalf("unexpected failed plan: %+v calls=%d", failedPlan.Report, len(failing.Calls()))
	}
	recovered.Store(true)
	retried, err := RetryFailed(context.Background(), failedPlan)
	if err != nil {
		t.Fatal(err)
	}
	if !retried.Complete || retried.Report.FailedJobs != 0 || len(retried.Issues) != 0 || len(failing.Calls()) != 4 {
		t.Fatalf("unexpected retried plan: %+v calls=%d", retried.Report, len(failing.Calls()))
	}
}

func TestRetryProgressCallbackCanInspectPlanWithoutDeadlock(t *testing.T) {
	input := filepath.Join(t.TempDir(), "retry-progress.xlsx")
	output := filepath.Join(t.TempDir(), "must-not-write.xlsx")
	date := time.Date(2026, time.August, 2, 0, 0, 0, 0, time.Local)
	createBatchWorkbook(t, input, []batchWorkbookRow{{store: "A", label: "A", date: date}})
	var recovered atomic.Bool
	provider := &batchTestProvider{handler: func(context.Context, rtasales.SalesQuery, int) (*rtasales.SalesResult, error) {
		if !recovered.Load() {
			return nil, &rtasales.UpstreamError{Operation: "sales", StatusCode: 503}
		}
		return batchResult(40, 4), nil
	}}
	inspect := atomic.Bool{}
	callbackResult := make(chan error, 1)
	var failedPlan Plan
	failedPlan, err := Analyze(context.Background(), provider, BatchRequest{
		InputPath: input, From: date, To: date,
		Backoff: func(context.Context, time.Duration) error { return nil },
		Progress: func(ProgressEvent) {
			if !inspect.Load() {
				return
			}
			_ = failedPlan.IsComplete()
			_, applyErr := Apply(context.Background(), failedPlan, ApplyRequest{OutputPath: output})
			var busy *PlanBusyError
			if !errors.As(applyErr, &busy) {
				callbackResult <- errors.New("re-entrant Apply did not return PlanBusyError")
				return
			}
			callbackResult <- nil
		},
	})
	if err != nil || failedPlan.Report.FailedJobs != 1 {
		t.Fatalf("initial analyze err=%v report=%+v", err, failedPlan.Report)
	}
	recovered.Store(true)
	inspect.Store(true)
	type retryResult struct {
		plan Plan
		err  error
	}
	done := make(chan retryResult, 1)
	go func() {
		plan, retryErr := RetryFailed(context.Background(), failedPlan)
		done <- retryResult{plan: plan, err: retryErr}
	}()
	select {
	case result := <-done:
		if result.err != nil || !result.plan.Complete {
			t.Fatalf("retry err=%v report=%+v", result.err, result.plan.Report)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RetryFailed deadlocked in a re-entrant progress callback")
	}
	if callbackErr := <-callbackResult; callbackErr != nil {
		t.Fatal(callbackErr)
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("re-entrant Apply wrote output: %v", err)
	}
}

func TestAnalyzeCancellationLeavesIncompletePlanThatCanResume(t *testing.T) {
	input := filepath.Join(t.TempDir(), "cancel.xlsx")
	date := time.Date(2026, time.August, 3, 0, 0, 0, 0, time.Local)
	createBatchWorkbook(t, input, []batchWorkbookRow{{store: "A", label: "A", date: date}})
	var recovered atomic.Bool
	started := make(chan struct{})
	provider := &batchTestProvider{handler: func(ctx context.Context, _ rtasales.SalesQuery, _ int) (*rtasales.SalesResult, error) {
		if recovered.Load() {
			return batchResult(50, 5), nil
		}
		select {
		case <-started:
		default:
			close(started)
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()
	plan, err := Analyze(ctx, provider, BatchRequest{InputPath: input, From: date, To: date})
	if !errors.Is(err, context.Canceled) || plan.Complete || plan.IsComplete() {
		t.Fatalf("err=%v complete=%t/%t", err, plan.Complete, plan.IsComplete())
	}
	_, err = Apply(context.Background(), plan, ApplyRequest{OutputPath: filepath.Join(t.TempDir(), "blocked.xlsx"), AllowPartial: true})
	var incomplete *IncompletePlanError
	if !errors.As(err, &incomplete) {
		t.Fatalf("error=%T %v, want IncompletePlanError", err, err)
	}
	recovered.Store(true)
	plan, err = plan.RetryFailed(context.Background())
	if err != nil || !plan.Complete || len(plan.Issues) != 0 {
		t.Fatalf("resume err=%v plan=%+v", err, plan.Report)
	}
}

type cancelOnceMapper struct {
	cancel context.CancelFunc
	once   sync.Once
}

func (mapper *cancelOnceMapper) ResolveStore(storeID string) (string, bool) {
	mapper.once.Do(mapper.cancel)
	return storeID, true
}

func TestRetryFailedRestartsAnInterruptedWorkbookScan(t *testing.T) {
	input := filepath.Join(t.TempDir(), "scan-cancel.xlsx")
	date := time.Date(2026, time.August, 3, 0, 0, 0, 0, time.Local)
	createBatchWorkbook(t, input, []batchWorkbookRow{
		{store: "A", label: "A", date: date},
		{store: "B", label: "B", date: date},
	})
	provider := &batchTestProvider{}
	ctx, cancel := context.WithCancel(context.Background())
	plan, err := Analyze(ctx, provider, BatchRequest{
		InputPath: input, From: date, To: date, Mapper: &cancelOnceMapper{cancel: cancel},
	})
	if !errors.Is(err, context.Canceled) || plan.Complete || len(provider.Calls()) != 0 {
		t.Fatalf("err=%v complete=%t calls=%d", err, plan.Complete, len(provider.Calls()))
	}
	plan, err = RetryFailed(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Complete || plan.Report.MatchedRows != 2 || plan.Report.UniqueQueries != 2 || len(provider.Calls()) != 2 {
		t.Fatalf("unexpected resumed scan: %+v calls=%d", plan.Report, len(provider.Calls()))
	}
}

func TestApplyProtectsIssuesSourceAndOutputAndPreservesWorkbook(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "fidelity.xlsx")
	output := filepath.Join(directory, "partial.xlsx")
	date := time.Date(2026, time.August, 4, 0, 0, 0, 0, time.Local)
	createBatchWorkbook(t, input, []batchWorkbookRow{
		{store: "A", label: "formula row", date: date},
		{store: "A", label: "ready row", date: date},
	})
	book, err := excelize.OpenFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellFormula(DefaultSheetName, "L2", "=1+1"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellFormula(DefaultSheetName, "M3", "=L3"); err != nil {
		t.Fatal(err)
	}
	style, err := book.NewStyle(&excelize.Style{NumFmt: 4, Fill: excelize.Fill{Type: "pattern", Color: []string{"#AABBCC"}, Pattern: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellStyle(DefaultSheetName, "L3", "AB3", style); err != nil {
		t.Fatal(err)
	}
	if err := book.MergeCell(DefaultSheetName, "A8", "B8"); err != nil {
		t.Fatal(err)
	}
	manual := "manual"
	no := false
	if err := book.SetCalcProps(&excelize.CalcPropsOptions{
		CalcMode: &manual, FullCalcOnLoad: &no, ForceFullCalc: &no, CalcOnSave: &no,
	}); err != nil {
		t.Fatal(err)
	}
	if err := book.Save(); err != nil {
		t.Fatal(err)
	}
	if err := book.Close(); err != nil {
		t.Fatal(err)
	}
	provider := &batchTestProvider{handler: func(context.Context, rtasales.SalesQuery, int) (*rtasales.SalesResult, error) {
		return batchResult(654321, 765), nil
	}}
	plan, err := Analyze(context.Background(), provider, BatchRequest{InputPath: input, From: date, To: date, Overwrite: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Issues) != 1 || plan.Issues[0].Code != "target_contains_formula" || plan.Rows[0].Status != RowStatusIssue || plan.Rows[1].Status != RowStatusReady {
		t.Fatalf("unexpected plan: %+v rows=%+v", plan.Report, plan.Rows)
	}
	strictOutput := filepath.Join(directory, "strict.xlsx")
	_, err = Apply(context.Background(), plan, ApplyRequest{OutputPath: strictOutput})
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error=%T %v, want ValidationError", err, err)
	}
	if _, err := os.Stat(strictOutput); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("strict output unexpectedly exists: %v", err)
	}
	_, err = Apply(context.Background(), plan, ApplyRequest{OutputPath: input, AllowPartial: true})
	var inputError *rtasales.InputError
	if !errors.As(err, &inputError) || inputError.Field != "OutputPath" {
		t.Fatalf("same-path error=%T %v", err, err)
	}
	hardlink := filepath.Join(directory, "source-hardlink.xlsx")
	if err := os.Link(input, hardlink); err != nil {
		t.Logf("hardlink output check skipped: %v", err)
	} else {
		_, err = Apply(context.Background(), plan, ApplyRequest{OutputPath: hardlink, AllowPartial: true})
		if !errors.As(err, &inputError) || inputError.Field != "OutputPath" {
			t.Fatalf("hardlink error=%T %v, want OutputPath InputError", err, err)
		}
	}
	if err := os.WriteFile(output, []byte("replace me"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := Apply(context.Background(), plan, ApplyRequest{OutputPath: output, AllowPartial: true})
	if err != nil {
		t.Fatal(err)
	}
	if !report.WroteWorkbook {
		t.Fatalf("workbook not reported written: %+v", report)
	}
	result, err := excelize.OpenFile(output, excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = result.Close() }()
	if formula, _ := result.GetCellFormula(DefaultSheetName, "L2"); formula != "=1+1" {
		t.Fatalf("formula row changed: %q", formula)
	}
	assertCellValue(t, result, "AB2", "")
	assertCellValue(t, result, "L3", "654321")
	assertCellValue(t, result, "AB3", "765")
	if formula, _ := result.GetCellFormula(DefaultSheetName, "M3"); formula != "=L3" {
		t.Fatalf("unrelated formula changed: %q", formula)
	}
	if got, err := result.GetCellStyle(DefaultSheetName, "L3"); err != nil || got != style {
		t.Fatalf("style=%d err=%v, want %d", got, err, style)
	}
	merges, err := result.GetMergeCells(DefaultSheetName)
	if err != nil {
		t.Fatal(err)
	}
	foundMerge := false
	for _, merge := range merges {
		if merge.GetStartAxis() == "A8" && merge.GetEndAxis() == "B8" {
			foundMerge = true
		}
	}
	if !foundMerge {
		t.Fatalf("merge A8:B8 not preserved: %+v", merges)
	}
	calc, err := result.GetCalcProps()
	if err != nil {
		t.Fatal(err)
	}
	if calc.CalcMode == nil || *calc.CalcMode != "manual" || boolValue(calc.FullCalcOnLoad) || boolValue(calc.ForceFullCalc) || boolValue(calc.CalcOnSave) {
		t.Fatalf("calculation settings changed: %+v", calc)
	}
}

func TestApplyCanForceWorkbookRecalculation(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "manual.xlsx")
	output := filepath.Join(directory, "recalculate.xlsx")
	date := time.Date(2026, time.August, 15, 0, 0, 0, 0, time.Local)
	createBatchWorkbook(t, input, []batchWorkbookRow{{store: "A", label: "Store A", date: date}})
	book, err := excelize.OpenFile(input)
	if err != nil {
		t.Fatal(err)
	}
	manual := "manual"
	no := false
	if err := book.SetCalcProps(&excelize.CalcPropsOptions{
		CalcMode: &manual, FullCalcOnLoad: &no, ForceFullCalc: &no, CalcOnSave: &no,
	}); err != nil {
		t.Fatal(err)
	}
	if err := book.Save(); err != nil {
		t.Fatal(err)
	}
	if err := book.Close(); err != nil {
		t.Fatal(err)
	}
	provider := &batchTestProvider{handler: func(context.Context, rtasales.SalesQuery, int) (*rtasales.SalesResult, error) {
		return batchResult(12, 3), nil
	}}
	plan, err := Analyze(context.Background(), provider, BatchRequest{InputPath: input, From: date, To: date})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(context.Background(), plan, ApplyRequest{
		OutputPath: output, ForceRecalculate: true,
	}); err != nil {
		t.Fatal(err)
	}
	result, err := excelize.OpenFile(output)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = result.Close() }()
	calc, err := result.GetCalcProps()
	if err != nil {
		t.Fatal(err)
	}
	if calc.CalcMode == nil || *calc.CalcMode != "auto" || !boolValue(calc.FullCalcOnLoad) || !boolValue(calc.ForceFullCalc) || !boolValue(calc.CalcOnSave) {
		t.Fatalf("calculation settings were not forced: %+v", calc)
	}
}

func TestAtomicPublishDoesNotTruncateHardlinkedSource(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "source.xlsx")
	destination := filepath.Join(directory, "destination.xlsx")
	temporary := filepath.Join(directory, "temporary.xlsx")
	if err := os.WriteFile(source, []byte("original source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(temporary, []byte("complete replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(source, destination); err != nil {
		t.Skipf("hardlink publication test unavailable: %v", err)
	}
	if err := replaceWorkbookFile(temporary, destination); err != nil {
		t.Fatal(err)
	}
	sourceBytes, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	destinationBytes, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(sourceBytes) != "original source" || string(destinationBytes) != "complete replacement" {
		t.Fatalf("unsafe publication: source=%q destination=%q", sourceBytes, destinationBytes)
	}
}

func boolValue(value *bool) bool { return value != nil && *value }

func TestApplyRejectsChangedSourceFingerprint(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "source.xlsx")
	output := filepath.Join(directory, "output.xlsx")
	date := time.Date(2026, time.August, 5, 0, 0, 0, 0, time.Local)
	createBatchWorkbook(t, input, []batchWorkbookRow{{store: "A", label: "A", date: date}})
	plan, err := Analyze(context.Background(), &batchTestProvider{}, BatchRequest{InputPath: input, From: date, To: date})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Source.SHA256 == "" || plan.Source.Size == 0 || plan.Source.ModTime.IsZero() {
		t.Fatalf("missing fingerprint: %+v", plan.Source)
	}
	changedTime := plan.Source.ModTime.Add(2 * time.Second)
	if err := os.Chtimes(input, changedTime, changedTime); err != nil {
		t.Fatal(err)
	}
	_, err = Apply(context.Background(), plan, ApplyRequest{OutputPath: output})
	var changed *SourceChangedError
	if !errors.As(err, &changed) {
		t.Fatalf("error=%T %v, want SourceChangedError", err, err)
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("output unexpectedly exists: %v", err)
	}
}

func TestAnalyzeOverwriteAndPlanJSONRedaction(t *testing.T) {
	input := filepath.Join(t.TempDir(), "SECRET-input.xlsx")
	date := time.Date(2026, time.August, 6, 0, 0, 0, 0, time.Local)
	createBatchWorkbook(t, input, []batchWorkbookRow{
		{store: "SECRET_STORE_9", label: "different", date: date, l: 1234567, ab: 123},
		{store: "SECRET_STORE_9", label: "same", date: date, l: 7654321, ab: 321},
	})
	provider := &batchTestProvider{handler: func(context.Context, rtasales.SalesQuery, int) (*rtasales.SalesResult, error) {
		return batchResult(7654321, 321), nil
	}}
	router, err := NewProfiledProviderRouter(map[string]ProviderRoute{
		"SECRET_STORE_9": {Provider: provider, Profile: "SECRET_PROFILE", Lane: "SECRET_LANE"},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := make([]ProgressEvent, 0)
	plan, err := Analyze(context.Background(), router, BatchRequest{
		InputPath: input, From: date, To: date,
		Progress: func(event ProgressEvent) { events = append(events, event) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Rows[0].Status != RowStatusIssue || plan.Rows[1].Status != RowStatusUnchanged || plan.Report.UnchangedCells != 2 || len(plan.Issues) != 1 || plan.Issues[0].Code != "existing_value_differs" {
		t.Fatalf("unexpected overwrite=false plan: %+v rows=%+v", plan.Report, plan.Rows)
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded) + plan.String()
	for _, secret := range []string{"SECRET_STORE_9", "SECRET_PROFILE", "SECRET_LANE", "SECRET-input", "1234567", "7654321"} {
		if strings.Contains(text, secret) {
			t.Fatalf("plan JSON leaked %q: %s", secret, text)
		}
	}
	if len(events) != 1 || events[0].Stage != "analyze" || events[0].CompletedJobs != 1 || events[0].TotalJobs != 1 || events[0].Profile != "SECRET_PROFILE" || events[0].StoreID != "SECRET_STORE_9" {
		t.Fatalf("unexpected progress events: %+v", events)
	}
	eventJSON, err := json.Marshal(events[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(eventJSON), "SECRET_PROFILE") || strings.Contains(string(eventJSON), "SECRET_STORE_9") {
		t.Fatalf("progress JSON leaked routing details: %s", eventJSON)
	}
	overwritePlan, err := Analyze(context.Background(), router, BatchRequest{
		InputPath: input, From: date, To: date, Overwrite: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if overwritePlan.Rows[0].Status != RowStatusReady || overwritePlan.Rows[1].Status != RowStatusUnchanged || overwritePlan.Report.StagedCells != 2 || overwritePlan.Report.UnchangedCells != 2 || len(overwritePlan.Issues) != 0 {
		t.Fatalf("unexpected overwrite=true plan: %+v rows=%+v", overwritePlan.Report, overwritePlan.Rows)
	}
}

func TestScanWorkbookReportsBoundsAndIntersectionWithoutNetwork(t *testing.T) {
	input := filepath.Join(t.TempDir(), "scan.xlsx")
	date := time.Date(2026, time.July, 31, 0, 0, 0, 0, time.Local)
	createBatchWorkbook(t, input, []batchWorkbookRow{
		{store: "A", label: "A", date: date},
		{store: "A", label: "A duplicate", date: date},
		{store: "B", label: "B", date: date.AddDate(0, 0, 1)},
		{store: "C", label: "C", date: date.AddDate(0, 0, 2)},
		{store: "D", label: "bad", date: "not-a-date"},
	})
	book, err := excelize.OpenFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := book.NewSheet("Other"); err != nil {
		t.Fatal(err)
	}
	if err := book.Save(); err != nil {
		t.Fatal(err)
	}
	if err := book.Close(); err != nil {
		t.Fatal(err)
	}
	scan, err := ScanWorkbook(input, DefaultSheetName, date, date.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(scan.Sheets)
	if scan.DateMin != "2026-07-31" || scan.DateMax != "2026-08-02" || scan.RowCount != 3 || scan.StoreCount != 2 || scan.JobCount != 2 || len(scan.Sheets) != 2 || len(scan.Issues) != 1 {
		t.Fatalf("unexpected scan: %+v", scan)
	}
}

func TestScanWorkbookContextHonorsPreCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ScanWorkbookContext(ctx, filepath.Join(t.TempDir(), "missing.xlsx"), DefaultSheetName, time.Time{}, time.Time{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%T %v, want context.Canceled", err, err)
	}
}

func TestAnalyzeEnforcesMaxJobsAndConcurrencyBounds(t *testing.T) {
	if DefaultMaxJobs != 2000 || DefaultConcurrency != 160 || MaximumConcurrency != 160 {
		t.Fatalf("unexpected defaults: jobs=%d concurrency=%d max=%d", DefaultMaxJobs, DefaultConcurrency, MaximumConcurrency)
	}
	input := filepath.Join(t.TempDir(), "limits.xlsx")
	date := time.Date(2026, time.August, 7, 0, 0, 0, 0, time.Local)
	createBatchWorkbook(t, input, []batchWorkbookRow{
		{store: "A", label: "A", date: date},
		{store: "B", label: "B", date: date},
	})
	_, err := Analyze(context.Background(), &batchTestProvider{}, BatchRequest{
		InputPath: input, From: date, To: date, MaxJobs: 1,
	})
	var inputError *rtasales.InputError
	if !errors.As(err, &inputError) || inputError.Field != "MaxJobs" {
		t.Fatalf("error=%T %v, want MaxJobs InputError", err, err)
	}
	_, err = Analyze(context.Background(), &batchTestProvider{}, BatchRequest{
		InputPath: input, From: date, To: date, Concurrency: MaximumConcurrency + 1,
	})
	if !errors.As(err, &inputError) || inputError.Field != "Concurrency" {
		t.Fatalf("error=%T %v, want Concurrency InputError", err, err)
	}
}
