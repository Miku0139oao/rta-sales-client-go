package desktop

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func tableWorkbookFixture() AnalysisWorkbookRequest {
	return AnalysisWorkbookRequest{Filename: "RTA-screen.xlsx", Context: []string{"帳號：測試", "2026-08-01 — 2026-08-31", "HKD"}, Sheets: []AnalysisTableSheet{{Name: "商品/分析", Columns: []AnalysisTableColumn{{Label: "商品編碼", Format: "text"}, {Label: "金額", Format: "money"}, {Label: "變化", Format: "percent"}}, Rows: [][]any{{"00107", float64(42.5), float64(.25)}, {"=HYPERLINK(\"bad\")", float64(-4), nil}}}}}
}
func TestAnalysisWorkbookTypesAndFormulaSafety(t *testing.T) {
	app := &App{}
	encoded, err := app.BuildSalesAnalysisWorkbook(tableWorkbookFixture())
	if err != nil {
		t.Fatal(err)
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sheets := f.GetSheetList()
	if len(sheets) != 2 {
		t.Fatalf("sheets=%v", sheets)
	}
	sheet := sheets[1]
	code, _ := f.GetCellValue(sheet, "A2")
	if code != "00107" {
		t.Fatalf("code=%q", code)
	}
	literal, _ := f.GetCellValue(sheet, "A3")
	formula, _ := f.GetCellFormula(sheet, "A3")
	if literal != "=HYPERLINK(\"bad\")" || formula != "" {
		t.Fatalf("literal=%q formula=%q", literal, formula)
	}
	kind, _ := f.GetCellType(sheet, "B2")
	// OOXML defaults an omitted type attribute to numeric.
	if kind != excelize.CellTypeNumber && kind != excelize.CellTypeUnset {
		t.Fatalf("money type=%v", kind)
	}
	money, _ := f.GetCellValue(sheet, "B2", excelize.Options{RawCellValue: true})
	if money != "42.5" {
		t.Fatalf("money=%q", money)
	}
	raw, _ := f.GetCellValue(sheet, "C2", excelize.Options{RawCellValue: true})
	if raw != "0.25" {
		t.Fatalf("percent=%q", raw)
	}
	panes, err := f.GetPanes(sheet)
	if err != nil || !panes.Freeze || panes.YSplit != 1 {
		t.Fatalf("panes=%+v %v", panes, err)
	}
}
func TestAnalysisWorkbookRejectsInvalidDimensionsAndCells(t *testing.T) {
	for _, mutate := range []func(*AnalysisWorkbookRequest){
		func(r *AnalysisWorkbookRequest) { r.Sheets = nil },
		func(r *AnalysisWorkbookRequest) { r.Sheets[0].Rows[0] = []any{"short"} },
		func(r *AnalysisWorkbookRequest) { r.Sheets[0].Rows[0][1] = math.NaN() },
		func(r *AnalysisWorkbookRequest) { r.Sheets[0].Rows[0][1] = map[string]any{"formula": "SUM(A1)"} },
		func(r *AnalysisWorkbookRequest) { r.Sheets[0].Columns[0].Format = "formula" },
	} {
		r := tableWorkbookFixture()
		mutate(&r)
		if _, err := buildAnalysisWorkbook(r); err == nil {
			t.Fatal("accepted invalid request")
		}
	}
}
func TestAnalysisWorkbookWebRPC(t *testing.T) {
	session := &webSession{app: &App{}}
	body, err := json.Marshal(tableWorkbookFixture())
	if err != nil {
		t.Fatal(err)
	}
	result, err := dispatchWebRPC(session, "BuildSalesAnalysisWorkbook", []json.RawMessage{body})
	if err != nil {
		t.Fatal(err)
	}
	encoded, ok := result.(string)
	if !ok {
		t.Fatalf("result type=%T", result)
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || !bytes.HasPrefix(data, []byte("PK")) {
		t.Fatalf("not an XLSX ZIP: %v", err)
	}
	if _, err := dispatchWebRPC(session, "BuildSalesAnalysisWorkbook", nil); err == nil {
		t.Fatal("accepted missing request")
	}
	if _, err := dispatchWebRPC(session, "ExportSalesAnalysisWorkbook", []json.RawMessage{body}); err == nil {
		t.Fatal("web RPC exposed native filesystem export")
	}
}

func TestExportAnalysisWorkbookCancellationAndNoOverwrite(t *testing.T) {
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{})
	app.dialogs = &fakeDialogs{}
	if path, err := app.ExportSalesAnalysisWorkbook(tableWorkbookFixture()); err != nil || path != "" {
		t.Fatalf("cancel=%q %v", path, err)
	}
	dir := t.TempDir()
	app.dialogs = &fakeDialogs{directory: dir}
	first, err := app.ExportSalesAnalysisWorkbook(tableWorkbookFixture())
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.ExportSalesAnalysisWorkbook(tableWorkbookFixture())
	if err != nil {
		t.Fatal(err)
	}
	if first == second || filepath.Base(second) != "RTA-screen-2.xlsx" {
		t.Fatalf("paths=%q %q", first, second)
	}
	if _, err := os.Stat(first); err != nil {
		t.Fatal(err)
	}
	r := tableWorkbookFixture()
	r.Filename = "../escape.xlsx"
	if _, err := app.ExportSalesAnalysisWorkbook(r); err == nil {
		t.Fatal("accepted traversal")
	}
}
