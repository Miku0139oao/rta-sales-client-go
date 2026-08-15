package desktop

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestChooseSalesAnalysisPDFDirectory(t *testing.T) {
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{})
	output := t.TempDir()
	app.dialogs = &fakeDialogs{directory: output}

	directory, err := app.ChooseSalesAnalysisPDFDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if directory != output {
		t.Fatalf("directory = %q, want %q", directory, output)
	}
}

func TestWriteSalesAnalysisPDFDoesNotOverwriteExistingReport(t *testing.T) {
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{})
	output := t.TempDir()
	data := []byte("%PDF-1.7\nstore report")
	request := SalesAnalysisPDFWriteRequest{
		Directory: output, Filename: "RTA-Sales-107.pdf",
		DataBase64: base64.StdEncoding.EncodeToString(data),
	}

	first, err := app.WriteSalesAnalysisPDF(request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.WriteSalesAnalysisPDF(request)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || filepath.Base(second) != "RTA-Sales-107-2.pdf" {
		t.Fatalf("existing PDF was not preserved: first=%q second=%q", first, second)
	}
	written, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	if string(written) != string(data) {
		t.Fatalf("written PDF changed: %q", written)
	}
}

func TestWriteSalesAnalysisPDFRejectsUnsafeInput(t *testing.T) {
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{})
	output := t.TempDir()
	valid := base64.StdEncoding.EncodeToString([]byte("%PDF-1.7\nreport"))
	tests := []SalesAnalysisPDFWriteRequest{
		{Filename: "report.pdf", DataBase64: valid},
		{Directory: output, Filename: "..\\escape.pdf", DataBase64: valid},
		{Directory: output, Filename: "report.txt", DataBase64: valid},
		{Directory: output, Filename: "report.pdf", DataBase64: base64.StdEncoding.EncodeToString([]byte("not a PDF"))},
	}
	for _, request := range tests {
		if _, err := app.WriteSalesAnalysisPDF(request); err == nil {
			t.Fatalf("unsafe request was accepted: %#v", request)
		}
	}
}
