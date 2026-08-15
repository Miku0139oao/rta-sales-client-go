package desktop

import (
	"testing"

	"github.com/Miku0139oao/rta-sales-client-go/xlsxfill"
)

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
