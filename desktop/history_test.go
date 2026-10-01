package desktop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReportHistoryPersistenceAndIntegrity(t *testing.T) {
	app, root, _ := newTestApp(t, new(fakeEngine), fakeClients{})
	report := HistoryReport{Version: 1, ID: "12345678-1234-1234-1234-123456789012", SavedAt: "2026-10-02T00:00:00Z", Name: "Report", From: "2026-09-01", To: "2026-09-30", Scope: "account:107", ScopeLabel: "107", Complete: true, Stores: []SalesAnalysisStoreSummary{}}
	if err := app.SaveReportHistory(report); err != nil {
		t.Fatal(err)
	}
	if err := app.SaveReportHistory(report); err == nil {
		t.Fatal("overwrote history")
	}
	restarted, _, _ := newTestAppAt(t, root, new(fakeEngine), fakeClients{})
	reports, err := restarted.ListReportHistory()
	if err != nil || len(reports) != 1 {
		t.Fatalf("history restart: %v %v", reports, err)
	}
	if err := restarted.DeleteReportHistory("../outside"); err == nil {
		t.Fatal("accepted traversal")
	}
	report.Complete = false
	report.ID = "12345678-1234-1234-1234-123456789013"
	if err := app.SaveReportHistory(report); err == nil {
		t.Fatal("saved incomplete report")
	}
	historyRoot, _ := app.historyRoot()
	if err := os.WriteFile(filepath.Join(historyRoot, "broken.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ListReportHistory(); err == nil {
		t.Fatal("silently discarded corrupt history")
	}
	if err := app.DeleteReportHistory(reports[0].ID); err != nil {
		t.Fatal(err)
	}
}
