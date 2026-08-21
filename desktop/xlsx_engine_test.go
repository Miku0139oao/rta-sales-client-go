package desktop

import (
	"testing"

	"github.com/Miku0139oao/rta-sales-client-go/xlsxfill"
)

func TestDesktopPlanKeepsQueryFailedRetryableAndSeparateFromPermission(t *testing.T) {
	plan := xlsxfill.Plan{
		Complete: true,
		Report: xlsxfill.Report{
			Complete: true, UniqueQueries: 3, CompletedJobs: 3, FailedJobs: 1,
			Issues: []xlsxfill.Issue{
				{Code: "query_failed", Rows: []int{3}},
				{Code: "store_not_authorized", Rows: []int{4}},
			},
		},
		Rows: []xlsxfill.RowPlan{
			{Row: 2, Date: "2026-08-01", WorkbookStoreID: "101", Status: xlsxfill.RowStatusReady},
			{Row: 3, Date: "2026-08-01", WorkbookStoreID: "102", Status: xlsxfill.RowStatusIssue, Issues: []string{"query_failed"}},
			{Row: 4, Date: "2026-08-01", WorkbookStoreID: "103", Status: xlsxfill.RowStatusIssue, Issues: []string{"store_not_authorized"}},
		},
	}
	converted := desktopPlan(plan, "plan")
	if converted.RetryableCount != 1 {
		t.Fatalf("retryableCount=%d, want 1 query_failed job", converted.RetryableCount)
	}
	if len(converted.Preview) != 3 {
		t.Fatalf("preview=%d, want 3", len(converted.Preview))
	}
	if converted.Preview[1].Status != "failed" || converted.Preview[1].Message != "query_failed" {
		t.Fatalf("query_failed row was not retryable failed: %#v", converted.Preview[1])
	}
	if converted.Preview[2].Status != "issue" || converted.Preview[2].Message != "store_not_authorized" {
		t.Fatalf("permission row mixed with query_failed: %#v", converted.Preview[2])
	}
}

func TestDesktopRowStatusTreatsQueryFailuresAsRetryableNotPermission(t *testing.T) {
	if got := desktopRowStatus(xlsxfill.RowPlan{Status: xlsxfill.RowStatusIssue, Issues: []string{"query_failed"}}); got != "failed" {
		t.Fatalf("query_failed status=%q, want failed", got)
	}
	if got := desktopRowStatus(xlsxfill.RowPlan{Status: xlsxfill.RowStatusIssue, Issues: []string{"upstream_error"}}); got != "failed" {
		t.Fatalf("upstream_error status=%q, want failed", got)
	}
	if got := desktopRowStatus(xlsxfill.RowPlan{Status: xlsxfill.RowStatusIssue, Issues: []string{"store_not_authorized"}}); got != "issue" {
		t.Fatalf("store_not_authorized status=%q, want issue", got)
	}
	converted := desktopPlan(xlsxfill.Plan{
		Report: xlsxfill.Report{UniqueQueries: 16, CompletedJobs: 16, FailedJobs: 2},
		Rows: []xlsxfill.RowPlan{
			{Row: 2, Date: "2026-08-08", Status: xlsxfill.RowStatusReady},
			{Row: 3, Date: "2026-08-08", Status: xlsxfill.RowStatusIssue, Issues: []string{"query_failed"}},
			{Row: 4, Date: "2026-08-08", Status: xlsxfill.RowStatusIssue, Issues: []string{"upstream_error"}},
			{Row: 5, Date: "2026-08-08", Status: xlsxfill.RowStatusIssue, Issues: []string{"store_not_authorized"}},
			{Row: 6, Date: "2026-08-08", Status: xlsxfill.RowStatusSkipped},
		},
	}, "plan")
	if converted.RetryableCount != 2 {
		t.Fatalf("RetryableCount=%d, want 2", converted.RetryableCount)
	}
	if len(converted.Preview) != 4 {
		t.Fatalf("preview=%d, want 4 non-skipped rows", len(converted.Preview))
	}
	statuses := map[int]string{}
	for _, row := range converted.Preview {
		statuses[row.Row] = row.Status
	}
	if statuses[2] != "change" || statuses[3] != "failed" || statuses[4] != "failed" || statuses[5] != "issue" {
		t.Fatalf("preview statuses=%v", statuses)
	}
}

func TestDesktopPlanExcludesNonProblemSkippedRows(t *testing.T) {
	plan := xlsxfill.Plan{
		Complete: true,
		Report:   xlsxfill.Report{Complete: true},
		Rows: []xlsxfill.RowPlan{
			{Row: 2, Date: "2026-08-01", WorkbookStoreID: "not-authorized", Status: xlsxfill.RowStatusSkipped},
			{Row: 3, Date: "2026-08-01", WorkbookStoreID: "ready", Status: xlsxfill.RowStatusReady},
		},
	}
	converted := desktopPlan(plan, "plan")
	if len(converted.Preview) != 1 || converted.Preview[0].Row != 3 {
		t.Fatalf("skipped row became a blocking preview issue: %#v", converted.Preview)
	}
}

func TestDisplayFormulaAddsExactlyOnePrefix(t *testing.T) {
	for input, want := range map[string]string{"1+1": "=1+1", "=1+1": "=1+1"} {
		if got := displayFormula(input); got != want {
			t.Fatalf("displayFormula(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestPreviewNumbersAreHumanReadable(t *testing.T) {
	value := 112734.18999999968
	if got := formatNumber(&value); got != "112734.19" {
		t.Fatalf("formatNumber() = %q, want %q", got, "112734.19")
	}
	for input, want := range map[string]string{
		"519.0000000001":       "519",
		" 80262.929999999993 ": "80262.93",
		"=SUM(A1:A2)":          "=SUM(A1:A2)",
	} {
		if got := formatCellNumber(input); got != want {
			t.Fatalf("formatCellNumber(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestEngineProgressPreservesLatestCompletedJob(t *testing.T) {
	converted := toEngineProgress(xlsxfill.ProgressEvent{
		CompletedJobs: 14, TotalJobs: 30, Date: "2026-08-07", StoreID: "107",
		Profile: "Production", Attempt: 2, Status: "success",
	})
	if converted.Completed != 14 || converted.Total != 30 || converted.Date != "2026-08-07" || converted.StoreID != "107" || converted.Profile != "Production" || converted.Attempt != 2 || converted.Status != "success" {
		t.Fatalf("progress fields were lost: %#v", converted)
	}
}
