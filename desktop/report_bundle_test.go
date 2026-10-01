package desktop

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"os"
	"testing"
)

func TestReportBundleValidationAndNoOverwrite(t *testing.T) {
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{})
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, _ := writer.Create("report.xlsx")
	entry.Write([]byte("report"))
	writer.Close()
	request := SalesAnalysisPDFWriteRequest{Directory: t.TempDir(), Filename: "RTA-report.zip", DataBase64: base64.StdEncoding.EncodeToString(buffer.Bytes())}
	first, err := app.WriteReportBundle(request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.WriteReportBundle(request)
	if err != nil || first == second {
		t.Fatalf("overwrote archive: %s %s %v", first, second, err)
	}
	if data, _ := os.ReadFile(first); !bytes.Equal(data, buffer.Bytes()) {
		t.Fatal("archive bytes changed")
	}
	request.Filename = "../escape.zip"
	if _, err := app.WriteReportBundle(request); err == nil {
		t.Fatal("accepted traversal")
	}
	request.Filename = "RTA-invalid.zip"
	request.DataBase64 = base64.StdEncoding.EncodeToString([]byte("invalid"))
	if _, err := app.WriteReportBundle(request); err == nil {
		t.Fatal("accepted invalid archive")
	}
}
