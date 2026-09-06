package desktop

import (
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"github.com/xuri/excelize/v2"
)

const maxAnalysisTableCells = 500000

type AnalysisTableColumn struct {
	Label  string `json:"label"`
	Format string `json:"format"`
}
type AnalysisTableSheet struct {
	Name    string                `json:"name"`
	Columns []AnalysisTableColumn `json:"columns"`
	Rows    [][]any               `json:"rows"`
}
type AnalysisWorkbookRequest struct {
	Filename string               `json:"filename"`
	Context  []string             `json:"context"`
	Sheets   []AnalysisTableSheet `json:"sheets"`
}

// BuildSalesAnalysisWorkbook renders only the supplied screen snapshot. It never queries RTA.
func (a *App) BuildSalesAnalysisWorkbook(request AnalysisWorkbookRequest) (string, error) {
	releaseAdmission, admissionErr := a.admitWork()
	if admissionErr != nil {
		return "", admissionErr
	}
	defer releaseAdmission()
	data, err := buildAnalysisWorkbook(request)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// ExportSalesAnalysisWorkbook uses a native directory choice and never overwrites a file.
func (a *App) ExportSalesAnalysisWorkbook(request AnalysisWorkbookRequest) (string, error) {
	releaseAdmission, admissionErr := a.admitWork()
	if admissionErr != nil {
		return "", admissionErr
	}
	defer releaseAdmission()
	name := strings.TrimSpace(request.Filename)
	if name == "" || len(name) > 180 || strings.ContainsAny(name, `/\:*?"<>|`) || filepath.Base(name) != name || !strings.EqualFold(filepath.Ext(name), ".xlsx") {
		return "", errors.New("invalid workbook filename")
	}
	data, err := buildAnalysisWorkbook(request)
	if err != nil {
		return "", err
	}
	directory, err := a.ChooseSalesAnalysisPDFDirectory()
	if err != nil || directory == "" {
		return "", err
	}
	return writeUniquePDF(directory, name, data)
}

func buildAnalysisWorkbook(request AnalysisWorkbookRequest) ([]byte, error) {
	validText := func(s string) bool { return len(utf16.Encode([]rune(s))) <= 32767 }
	if len(request.Sheets) == 0 || len(request.Sheets) > 32 || len(request.Context) > 128 {
		return nil, errors.New("invalid workbook dimensions")
	}
	cells := 0
	for _, line := range request.Context {
		if !validText(line) {
			return nil, errors.New("context is too long")
		}
	}
	for _, sheet := range request.Sheets {
		if len(sheet.Columns) == 0 || len(sheet.Columns) > 32 || len(sheet.Rows) > 100000 || strings.TrimSpace(sheet.Name) == "" || len(sheet.Name) > 512 {
			return nil, errors.New("invalid sheet dimensions")
		}
		cells += len(sheet.Rows) * len(sheet.Columns)
		if cells > maxAnalysisTableCells {
			return nil, errors.New("workbook exceeds cell limit")
		}
		for _, col := range sheet.Columns {
			if !validText(col.Label) || col.Label == "" || (col.Format != "text" && col.Format != "number" && col.Format != "money" && col.Format != "percent") {
				return nil, errors.New("invalid column")
			}
		}
		for _, row := range sheet.Rows {
			if len(row) != len(sheet.Columns) {
				return nil, errors.New("row width mismatch")
			}
			for _, cell := range row {
				switch value := cell.(type) {
				case nil:
				case string:
					if !validText(value) {
						return nil, errors.New("cell text is too long")
					}
				case float64:
					if math.IsNaN(value) || math.IsInf(value, 0) {
						return nil, errors.New("non-finite number")
					}
				default:
					return nil, errors.New("unsupported cell value")
				}
			}
		}
	}
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetName("Sheet1", "Report"); err != nil {
		return nil, err
	}
	if err := f.SetColWidth("Report", "A", "A", 100); err != nil {
		return nil, err
	}
	for index, line := range request.Context {
		if err := f.SetCellStr("Report", fmt.Sprintf("A%d", index+1), line); err != nil {
			return nil, err
		}
	}
	header, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: "FFFFFF"}, Fill: excelize.Fill{Type: "pattern", Color: []string{"087F78"}, Pattern: 1}})
	if err != nil {
		return nil, err
	}
	styles := map[string]int{}
	for name, format := range map[string]string{"text": "@", "number": "#,##0.##", "money": "#,##0.00", "percent": "0.0%"} {
		id, err := f.NewStyle(&excelize.Style{CustomNumFmt: &format})
		if err != nil {
			return nil, err
		}
		styles[name] = id
	}
	for index, sheet := range request.Sheets {
		clean := strings.Map(func(r rune) rune {
			if strings.ContainsRune(`[]:*?/\`, r) || r < 32 {
				return '_'
			}
			return r
		}, sheet.Name)
		name := fmt.Sprintf("%02d_", index+1)
		for _, r := range clean {
			if len(utf16.Encode([]rune(name+string(r)))) > 31 {
				break
			}
			name += string(r)
		}
		name = strings.TrimRight(name, "'")
		if _, err := f.NewSheet(name); err != nil {
			return nil, err
		}
		for colIndex, col := range sheet.Columns {
			letter, _ := excelize.ColumnNumberToName(colIndex + 1)
			if err := f.SetCellStr(name, letter+"1", col.Label); err != nil {
				return nil, err
			}
			if len(sheet.Rows) > 0 {
				if err := f.SetCellStyle(name, letter+"2", fmt.Sprintf("%s%d", letter, len(sheet.Rows)+1), styles[col.Format]); err != nil {
					return nil, err
				}
			}
			width := 18.0
			if col.Format == "text" {
				width = 30
			}
			if err := f.SetColWidth(name, letter, letter, width); err != nil {
				return nil, err
			}
		}
		lastCol, _ := excelize.ColumnNumberToName(len(sheet.Columns))
		if err := f.SetCellStyle(name, "A1", lastCol+"1", header); err != nil {
			return nil, err
		}
		for rowIndex, row := range sheet.Rows {
			for colIndex, cell := range row {
				axis, _ := excelize.CoordinatesToCellName(colIndex+1, rowIndex+2)
				var err error
				if value, ok := cell.(string); ok {
					err = f.SetCellStr(name, axis, value)
				} else {
					err = f.SetCellValue(name, axis, cell)
				}
				if err != nil {
					return nil, err
				}
			}
		}
		if err := f.SetPanes(name, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
			return nil, err
		}
		if err := f.AutoFilter(name, fmt.Sprintf("A1:%s%d", lastCol, len(sheet.Rows)+1), nil); err != nil {
			return nil, err
		}
	}
	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
