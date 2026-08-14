package xlsxfill

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	rtasales "github.com/Miku0139oao/rta-sales-client-go"
	"github.com/xuri/excelize/v2"
)

type fakeSalesProvider struct {
	results map[string]*rtasales.SalesResult
	errors  map[string]error
	calls   []rtasales.SalesQuery
}

func (provider *fakeSalesProvider) Sales(_ context.Context, query rtasales.SalesQuery) (*rtasales.SalesResult, error) {
	provider.calls = append(provider.calls, query)
	if err := provider.errors[query.BusinessStoreID]; err != nil {
		return nil, err
	}
	return provider.results[query.BusinessStoreID], nil
}

func TestFillWritesOnlyManualInputsAndPreservesFormulasAndStyles(t *testing.T) {
	input := filepath.Join(t.TempDir(), "input.xlsx")
	output := filepath.Join(t.TempDir(), "output.xlsx")
	targetDate := time.Date(2026, time.August, 13, 0, 0, 0, 0, time.Local)
	wantLStyle, wantABStyle := createTestWorkbook(t, input, targetDate)
	countA := 431.0
	countB := 10.0
	provider := &fakeSalesProvider{results: map[string]*rtasales.SalesResult{
		"RTA_A": {
			TotalAmount:           52374,
			TotalTransactionCount: &countA,
			Items:                 []rtasales.SaleItem{{Matnr: "ITEM_A"}},
		},
		"RTA_B": {
			TotalAmount:           100,
			TotalTransactionCount: &countB,
			Items:                 []rtasales.SaleItem{{Matnr: "ITEM_B"}},
		},
	}}
	report, err := Fill(context.Background(), provider, Request{
		InputPath:  input,
		OutputPath: output,
		Date:       targetDate,
		Mapper:     StoreMap{"STORE_A": "RTA_A", "STORE_B": "RTA_B"},
		Write:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.WroteWorkbook || report.MatchedRows != 2 || report.SelectedRows != 2 || report.SkippedStoreRows != 0 || report.UniqueQueries != 2 || report.StagedCells != 2 || report.UnchangedCells != 2 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if len(provider.calls) != 2 {
		t.Fatalf("queries=%d, want 2", len(provider.calls))
	}
	book, err := excelize.OpenFile(output, excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = book.Close() }()
	assertCellValue(t, book, "L2", "52374")
	assertCellValue(t, book, "AB2", "431")
	assertCellValue(t, book, "L3", "999")
	assertCellValue(t, book, "AB3", "99")
	assertCellValue(t, book, "L4", "100")
	assertCellValue(t, book, "AB4", "10")
	assertCellValue(t, book, "L5", "")
	assertCellValue(t, book, "AB5", "")
	if formula, err := book.GetCellFormula(DefaultSheetName, "M2"); err != nil || formula != "=L2" {
		t.Fatalf("M2 formula=%q err=%v, want =L2", formula, err)
	}
	if formula, err := book.GetCellFormula(DefaultSheetName, "AD2"); err != nil || formula != "=AB2" {
		t.Fatalf("AD2 formula=%q err=%v, want =AB2", formula, err)
	}
	if style, err := book.GetCellStyle(DefaultSheetName, "L2"); err != nil || style != wantLStyle {
		t.Fatalf("L2 style=%d err=%v, want %d", style, err, wantLStyle)
	}
	if style, err := book.GetCellStyle(DefaultSheetName, "AB2"); err != nil || style != wantABStyle {
		t.Fatalf("AB2 style=%d err=%v, want %d", style, err, wantABStyle)
	}
	calc, err := book.GetCalcProps()
	if err != nil {
		t.Fatal(err)
	}
	if calc.CalcMode == nil || *calc.CalcMode != "auto" || calc.FullCalcOnLoad == nil || !*calc.FullCalcOnLoad || calc.ForceFullCalc == nil || !*calc.ForceFullCalc {
		t.Fatalf("calculation properties were not set for recalculation: %+v", calc)
	}
}

func TestFillAutomaticallySelectsAuthorizedStores(t *testing.T) {
	input := filepath.Join(t.TempDir(), "input.xlsx")
	targetDate := time.Date(2026, time.August, 13, 0, 0, 0, 0, time.Local)
	createTestWorkbook(t, input, targetDate)
	count := 11.0
	provider := &fakeSalesProvider{results: map[string]*rtasales.SalesResult{
		"RTA_B": {TotalAmount: 101, TotalTransactionCount: &count, Items: []rtasales.SaleItem{{Matnr: "ITEM_B"}}},
	}}
	report, err := Fill(context.Background(), provider, Request{
		InputPath:               input,
		Date:                    targetDate,
		Mapper:                  StoreMap{"STORE_A": "RTA_A", "STORE_B": "RTA_B"},
		AllowedBusinessStoreIDs: []string{"RTA_B"},
		Overwrite:               true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.MatchedRows != 2 || report.SelectedRows != 1 || report.SkippedStoreRows != 1 || report.UniqueQueries != 1 || report.StagedCells != 2 {
		t.Fatalf("unexpected automatic selection report: %+v", report)
	}
	if len(provider.calls) != 1 || provider.calls[0].BusinessStoreID != "RTA_B" {
		t.Fatalf("unexpected provider calls: %+v", provider.calls)
	}
}

func TestFillRejectsWhenNoAuthorizedStoreMatches(t *testing.T) {
	input := filepath.Join(t.TempDir(), "input.xlsx")
	targetDate := time.Date(2026, time.August, 13, 0, 0, 0, 0, time.Local)
	createTestWorkbook(t, input, targetDate)
	provider := &fakeSalesProvider{}
	report, err := Fill(context.Background(), provider, Request{
		InputPath:               input,
		Date:                    targetDate,
		Mapper:                  StoreMap{"STORE_A": "RTA_A", "STORE_B": "RTA_B"},
		AllowedBusinessStoreIDs: []string{"RTA_C"},
	})
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error=%T %v, want ValidationError", err, err)
	}
	if report.MatchedRows != 2 || report.SelectedRows != 0 || report.SkippedStoreRows != 2 || report.UniqueQueries != 0 || len(provider.calls) != 0 {
		t.Fatalf("unexpected no-match report: %+v calls=%+v", report, provider.calls)
	}
	if len(report.Issues) != 1 || report.Issues[0].Code != "no_authorized_store_match" || len(report.Issues[0].Rows) != 2 {
		t.Fatalf("unexpected no-match issues: %+v", report.Issues)
	}
}

func TestFillStrictModeDoesNotWritePartialWorkbook(t *testing.T) {
	input := filepath.Join(t.TempDir(), "input.xlsx")
	output := filepath.Join(t.TempDir(), "output.xlsx")
	targetDate := time.Date(2026, time.August, 13, 0, 0, 0, 0, time.Local)
	createTestWorkbook(t, input, targetDate)
	count := 431.0
	provider := &fakeSalesProvider{results: map[string]*rtasales.SalesResult{
		"RTA_A": {TotalAmount: 52374, TotalTransactionCount: &count, Items: []rtasales.SaleItem{{Matnr: "ITEM_A"}}},
	}}
	report, err := Fill(context.Background(), provider, Request{
		InputPath:  input,
		OutputPath: output,
		Date:       targetDate,
		Mapper:     StoreMap{"STORE_A": "RTA_A"},
		Write:      true,
	})
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error=%T %v, want ValidationError", err, err)
	}
	if len(report.Issues) != 1 || report.Issues[0].Code != "missing_mapping" {
		t.Fatalf("unexpected report issues: %+v", report.Issues)
	}
	if _, statErr := os.Stat(output); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("partial output exists or stat failed: %v", statErr)
	}
}

func TestFillDoesNotReplaceDifferentExistingValueWithoutOverwrite(t *testing.T) {
	input := filepath.Join(t.TempDir(), "input.xlsx")
	targetDate := time.Date(2026, time.August, 13, 0, 0, 0, 0, time.Local)
	createTestWorkbook(t, input, targetDate)
	countA := 431.0
	countB := 11.0
	provider := &fakeSalesProvider{results: map[string]*rtasales.SalesResult{
		"RTA_A": {TotalAmount: 52374, TotalTransactionCount: &countA, Items: []rtasales.SaleItem{{Matnr: "ITEM_A"}}},
		"RTA_B": {TotalAmount: 101, TotalTransactionCount: &countB, Items: []rtasales.SaleItem{{Matnr: "ITEM_B"}}},
	}}
	report, err := Fill(context.Background(), provider, Request{
		InputPath: input,
		Date:      targetDate,
		Mapper:    StoreMap{"STORE_A": "RTA_A", "STORE_B": "RTA_B"},
	})
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error=%T %v, want ValidationError", err, err)
	}
	if len(report.Issues) != 1 || report.Issues[0].Code != "existing_value_differs" || len(report.Issues[0].Rows) != 1 || report.Issues[0].Rows[0] != 4 {
		t.Fatalf("unexpected report issues: %+v", report.Issues)
	}
}

func TestFillRequiresConsistentTransactionAggregate(t *testing.T) {
	input := filepath.Join(t.TempDir(), "input.xlsx")
	targetDate := time.Date(2026, time.August, 13, 0, 0, 0, 0, time.Local)
	createTestWorkbook(t, input, targetDate)
	provider := &fakeSalesProvider{results: map[string]*rtasales.SalesResult{
		"RTA_A": {TotalAmount: 52374, Items: []rtasales.SaleItem{{Matnr: "ITEM_A"}}},
		"RTA_B": {TotalAmount: 100, Items: []rtasales.SaleItem{{Matnr: "ITEM_B"}}},
	}}
	report, err := Fill(context.Background(), provider, Request{
		InputPath: input,
		Date:      targetDate,
		Mapper:    StoreMap{"STORE_A": "RTA_A", "STORE_B": "RTA_B"},
	})
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error=%T %v, want ValidationError", err, err)
	}
	if len(report.Issues) != 1 || report.Issues[0].Code != "transaction_total_unavailable" {
		t.Fatalf("unexpected report issues: %+v", report.Issues)
	}
}

func TestFillRejectsNonFiniteSalesTotal(t *testing.T) {
	input := filepath.Join(t.TempDir(), "input.xlsx")
	targetDate := time.Date(2026, time.August, 13, 0, 0, 0, 0, time.Local)
	createTestWorkbook(t, input, targetDate)
	count := 431.0
	provider := &fakeSalesProvider{results: map[string]*rtasales.SalesResult{
		"RTA_A": {TotalAmount: math.NaN(), TotalTransactionCount: &count, Items: []rtasales.SaleItem{{Matnr: "ITEM_A"}}},
	}}
	report, err := Fill(context.Background(), provider, Request{
		InputPath:  input,
		Date:       targetDate,
		Mapper:     StoreMap{"STORE_A": "RTA_A"},
		OnlyRow:    2,
		MaxQueries: 1,
	})
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error=%T %v, want ValidationError", err, err)
	}
	if len(report.Issues) != 1 || report.Issues[0].Code != "invalid_sales_total" {
		t.Fatalf("unexpected report issues: %+v", report.Issues)
	}
}

func TestFillRejectsSourceAsOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.xlsx")
	_, err := Fill(context.Background(), &fakeSalesProvider{}, Request{
		InputPath:  path,
		OutputPath: path,
		Date:       time.Date(2026, time.August, 13, 0, 0, 0, 0, time.Local),
		Write:      true,
	})
	var inputError *rtasales.InputError
	if !errors.As(err, &inputError) || inputError.Field != "OutputPath" {
		t.Fatalf("error=%T %v, want OutputPath InputError", err, err)
	}
}

func TestFillPreservesFormulaInTargetCell(t *testing.T) {
	input := filepath.Join(t.TempDir(), "input.xlsx")
	output := filepath.Join(t.TempDir(), "output.xlsx")
	targetDate := time.Date(2026, time.August, 13, 0, 0, 0, 0, time.Local)
	createTestWorkbook(t, input, targetDate)
	book, err := excelize.OpenFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellFormula(DefaultSheetName, "L2", "=1+1"); err != nil {
		t.Fatal(err)
	}
	if err := book.Save(); err != nil {
		t.Fatal(err)
	}
	if err := book.Close(); err != nil {
		t.Fatal(err)
	}

	count := 431.0
	provider := &fakeSalesProvider{results: map[string]*rtasales.SalesResult{
		"STORE_A": {TotalAmount: 52374, TotalTransactionCount: &count, Items: []rtasales.SaleItem{{Matnr: "ITEM_A"}}},
	}}
	report, err := Fill(context.Background(), provider, Request{
		InputPath:    input,
		OutputPath:   output,
		Date:         targetDate,
		Write:        true,
		AllowPartial: true,
		OnlyRow:      2,
		MaxQueries:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.StagedCells != 1 || len(report.Issues) != 1 || report.Issues[0].Code != "target_contains_formula" {
		t.Fatalf("unexpected report: %+v", report)
	}
	result, err := excelize.OpenFile(output, excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = result.Close() }()
	if formula, err := result.GetCellFormula(DefaultSheetName, "L2"); err != nil || formula != "=1+1" {
		t.Fatalf("L2 formula=%q err=%v, want =1+1", formula, err)
	}
	assertCellValue(t, result, "AB2", "431")
}

func TestFillAllowsSafePartialOutputWhenExplicitlyRequested(t *testing.T) {
	input := filepath.Join(t.TempDir(), "input.xlsx")
	output := filepath.Join(t.TempDir(), "output.xlsx")
	targetDate := time.Date(2026, time.August, 13, 0, 0, 0, 0, time.Local)
	createTestWorkbook(t, input, targetDate)
	count := 431.0
	provider := &fakeSalesProvider{results: map[string]*rtasales.SalesResult{
		"RTA_A": {TotalAmount: 52374, TotalTransactionCount: &count, Items: []rtasales.SaleItem{{Matnr: "ITEM_A"}}},
	}}
	report, err := Fill(context.Background(), provider, Request{
		InputPath:    input,
		OutputPath:   output,
		Date:         targetDate,
		Mapper:       StoreMap{"STORE_A": "RTA_A"},
		Write:        true,
		AllowPartial: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.WroteWorkbook || report.StagedCells != 2 || len(report.Issues) != 1 {
		t.Fatalf("unexpected partial report: %+v", report)
	}
}

func TestFillOnlyRowBoundsLiveSmokeTest(t *testing.T) {
	input := filepath.Join(t.TempDir(), "input.xlsx")
	targetDate := time.Date(2026, time.August, 13, 0, 0, 0, 0, time.Local)
	createTestWorkbook(t, input, targetDate)
	count := 431.0
	provider := &fakeSalesProvider{results: map[string]*rtasales.SalesResult{
		"STORE_A": {TotalAmount: 52374, TotalTransactionCount: &count, Items: []rtasales.SaleItem{{Matnr: "ITEM_A"}}},
	}}
	report, err := Fill(context.Background(), provider, Request{
		InputPath:  input,
		Date:       targetDate,
		OnlyRow:    2,
		MaxQueries: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.MatchedRows != 1 || report.UniqueQueries != 1 || report.StagedCells != 2 || len(provider.calls) != 1 {
		t.Fatalf("unexpected bounded report: %+v calls=%d", report, len(provider.calls))
	}
}

func createTestWorkbook(t *testing.T, path string, targetDate time.Time) (int, int) {
	t.Helper()
	book := excelize.NewFile()
	defer func() { _ = book.Close() }()
	if err := book.SetSheetName("Sheet1", DefaultSheetName); err != nil {
		t.Fatal(err)
	}
	for cell, value := range map[string]any{
		"C1": "Store ID", "E1": "Store ABR", "F1": "Date", "L1": "Daily Sales", "M1": "MTD",
		"AB1": "Customer Count", "AD1": "MTD Customer Count",
		"C2": "STORE_A", "E2": "AA", "F2": targetDate,
		"C3": "", "E3": "Total", "L3": 999, "AB3": 99,
		"C4": "STORE_B", "E4": "BB", "F4": targetDate, "L4": 100, "AB4": 10,
		"C5": "STORE_A", "E5": "AA", "F5": targetDate.AddDate(0, 0, 1),
	} {
		if err := book.SetCellValue(DefaultSheetName, cell, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := book.SetCellFormula(DefaultSheetName, "M2", "=L2"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellFormula(DefaultSheetName, "AD2", "=AB2"); err != nil {
		t.Fatal(err)
	}
	dateStyle, err := book.NewStyle(&excelize.Style{NumFmt: 14})
	if err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellStyle(DefaultSheetName, "F2", "F5", dateStyle); err != nil {
		t.Fatal(err)
	}
	lStyle, err := book.NewStyle(&excelize.Style{NumFmt: 4, Fill: excelize.Fill{Type: "pattern", Color: []string{"#FFF2CC"}, Pattern: 1}})
	if err != nil {
		t.Fatal(err)
	}
	abStyle, err := book.NewStyle(&excelize.Style{NumFmt: 3, Fill: excelize.Fill{Type: "pattern", Color: []string{"#DDEBF7"}, Pattern: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellStyle(DefaultSheetName, "L2", "L2", lStyle); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellStyle(DefaultSheetName, "AB2", "AB2", abStyle); err != nil {
		t.Fatal(err)
	}
	if err := book.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	return lStyle, abStyle
}

func assertCellValue(t *testing.T, book *excelize.File, cell, want string) {
	t.Helper()
	got, err := book.GetCellValue(DefaultSheetName, cell, excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s=%q, want %q", cell, got, want)
	}
}
