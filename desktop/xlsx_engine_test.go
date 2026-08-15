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
