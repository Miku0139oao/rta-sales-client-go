package desktop

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Miku0139oao/rta-sales-client-go/xlsxfill"
)

type xlsxEngine struct{}

func newXLSXEngine() batchEngine {
	return xlsxEngine{}
}

func (xlsxEngine) Scan(request engineScanRequest) (WorkbookScan, error) {
	scan, err := xlsxfill.ScanWorkbook(request.InputPath, request.Sheet, request.From, request.To)
	if err != nil {
		return WorkbookScan{}, err
	}
	result := WorkbookScan{
		InputPath: request.InputPath, SheetName: scan.Sheet,
		DateMin: scan.DateMin, DateMax: scan.DateMax,
		RowCount: scan.RowCount, StoreCount: scan.StoreCount, JobCount: scan.JobCount,
		Rows: scan.RowCount, Stores: scan.StoreCount, Jobs: scan.JobCount,
	}
	result.Sheets = make([]SheetSummary, 0, len(scan.Sheets))
	for _, name := range scan.Sheets {
		summary := SheetSummary{Name: name}
		if name == scan.Sheet {
			summary.DateMin, summary.DateMax, summary.Rows = scan.DateMin, scan.DateMax, scan.RowCount
		}
		result.Sheets = append(result.Sheets, summary)
	}
	result.Dates = inclusiveDateStrings(scan.DateMin, scan.DateMax)
	for _, issue := range scan.Issues {
		result.Warnings = append(result.Warnings, issue.Code)
	}
	return result, nil
}

func (xlsxEngine) Analyze(ctx context.Context, provider xlsxfill.SalesProvider, request engineAnalyzeRequest) (*enginePlan, error) {
	planID, err := newUUID()
	if err != nil {
		return nil, err
	}
	plan, analyzeErr := xlsxfill.Analyze(ctx, provider, xlsxfill.BatchRequest{
		InputPath: request.InputPath, SheetName: request.Sheet,
		From: request.From, To: request.To, Mapper: request.Mapper,
		AllowedBusinessStoreIDs: request.AllowedBusinessStoreIDs,
		Overwrite:               request.Overwrite, MaxJobs: request.MaxJobs, Concurrency: request.Concurrency,
		Progress: func(event xlsxfill.ProgressEvent) {
			if request.Progress != nil {
				request.Progress(engineProgress{Completed: event.CompletedJobs, Total: event.TotalJobs, Status: event.Status})
			}
		},
	})
	converted := desktopPlan(plan, planID)
	if analyzeErr != nil {
		// Cancellation intentionally preserves the in-memory plan so the user can
		// resume its pending jobs with RetryFailed.
		if (errors.Is(analyzeErr, context.Canceled) || errors.Is(analyzeErr, context.DeadlineExceeded)) && plan.InputPath != "" {
			return converted, nil
		}
		return converted, analyzeErr
	}
	return converted, nil
}

func (xlsxEngine) RetryFailed(ctx context.Context, existing *enginePlan, progress func(engineProgress)) (*enginePlan, error) {
	plan, ok := existing.Handle.(xlsxfill.Plan)
	if !ok {
		return nil, errors.New("workbook plan is not an xlsxfill plan")
	}
	updated, retryErr := xlsxfill.RetryFailed(ctx, plan)
	converted := desktopPlan(updated, existing.PlanID)
	if retryErr != nil {
		if (errors.Is(retryErr, context.Canceled) || errors.Is(retryErr, context.DeadlineExceeded)) && updated.InputPath != "" {
			return converted, nil
		}
		return converted, retryErr
	}
	if progress != nil {
		progress(engineProgress{Completed: updated.Report.CompletedJobs, Total: updated.Report.UniqueQueries, Status: "complete"})
	}
	return converted, nil
}

func (xlsxEngine) Apply(ctx context.Context, existing *enginePlan, outputPath string, allowPartial bool) (engineApplyResult, error) {
	plan, ok := existing.Handle.(xlsxfill.Plan)
	if !ok {
		return engineApplyResult{}, errors.New("workbook plan is not an xlsxfill plan")
	}
	report, err := xlsxfill.Apply(ctx, plan, xlsxfill.ApplyRequest{
		OutputPath: outputPath, AllowPartial: allowPartial, ForceRecalculate: true,
	})
	return engineApplyResult{
		Complete: report.Complete, ProblemCount: reportProblemCount(report),
		ChangedCellCount: report.StagedCells, WroteWorkbook: report.WroteWorkbook,
	}, err
}

func desktopPlan(plan xlsxfill.Plan, planID string) *enginePlan {
	result := &enginePlan{
		Handle: plan, PlanID: planID, InputPath: plan.InputPath, Complete: plan.Complete,
		ProblemCount: reportProblemCount(plan.Report), ChangedCellCount: plan.Report.StagedCells,
	}
	pending := plan.Report.UniqueQueries - plan.Report.CompletedJobs
	if pending < 0 {
		pending = 0
	}
	result.RetryableCount = plan.Report.FailedJobs + pending
	result.Preview = make([]PreviewRow, 0, len(plan.Rows))
	for _, row := range plan.Rows {
		// Ordinary unauthorized/skipped rows are informational in the core
		// report, not problems, so they must not appear as blocking UI issues.
		if row.Status == xlsxfill.RowStatusSkipped {
			continue
		}
		status := desktopRowStatus(row)
		currentL := row.Current.DailySales
		if currentL == "" && row.Current.DailySalesFormula != "" {
			currentL = displayFormula(row.Current.DailySalesFormula)
		}
		currentAB := row.Current.TransactionCount
		if currentAB == "" && row.Current.TransactionCountFormula != "" {
			currentAB = displayFormula(row.Current.TransactionCountFormula)
		}
		preview := PreviewRow{
			ID: fmt.Sprintf("%s:%d", row.Date, row.Row), Date: row.Date, Row: row.Row,
			WorkbookStoreID: row.WorkbookStoreID, StoreLabel: row.WorkbookStoreID,
			ProfileDisplayName: row.Profile, ProfileLabel: row.Profile,
			CurrentL: currentL, CurrentAB: currentAB,
			ProposedL: formatNumber(row.Proposed.DailySales), ProposedAB: formatNumber(row.Proposed.TransactionCount),
			Status: status, IssueCodes: append([]string(nil), row.Issues...),
		}
		if len(preview.IssueCodes) > 0 {
			preview.Message = preview.IssueCodes[0]
		}
		result.Preview = append(result.Preview, preview)
	}
	representedProblems := 0
	for _, row := range result.Preview {
		if row.Status == "issue" || row.Status == "failed" {
			representedProblems++
		}
	}
	result.AggregateProblemCount = result.ProblemCount - representedProblems
	if result.AggregateProblemCount < 0 {
		result.AggregateProblemCount = 0
	}
	return result
}

func desktopRowStatus(row xlsxfill.RowPlan) string {
	switch row.Status {
	case xlsxfill.RowStatusReady:
		return "change"
	case xlsxfill.RowStatusUnchanged:
		return "unchanged"
	case xlsxfill.RowStatusPending:
		return "failed"
	case xlsxfill.RowStatusIssue:
		for _, issue := range row.Issues {
			if issue == "upstream_error" || issue == "query_failed" {
				return "failed"
			}
		}
		return "issue"
	default:
		return "issue"
	}
}

func formatNumber(value *float64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(*value, 'f', -1, 64)
}

func displayFormula(formula string) string {
	if strings.HasPrefix(formula, "=") {
		return formula
	}
	return "=" + formula
}

func reportProblemCount(report xlsxfill.Report) int {
	rows := make(map[int]struct{})
	withoutRows := 0
	for _, issue := range report.Issues {
		if len(issue.Rows) == 0 {
			withoutRows++
			continue
		}
		for _, row := range issue.Rows {
			rows[row] = struct{}{}
		}
	}
	return len(rows) + withoutRows
}

func inclusiveDateStrings(minimum, maximum string) []string {
	if strings.TrimSpace(minimum) == "" || strings.TrimSpace(maximum) == "" {
		return []string{}
	}
	from, err := time.Parse("2006-01-02", minimum)
	if err != nil {
		return []string{}
	}
	to, err := time.Parse("2006-01-02", maximum)
	if err != nil || to.Before(from) {
		return []string{}
	}
	result := make([]string, 0, int(to.Sub(from)/(24*time.Hour))+1)
	for date := from; !date.After(to); date = date.AddDate(0, 0, 1) {
		result = append(result, date.Format("2006-01-02"))
	}
	return result
}
