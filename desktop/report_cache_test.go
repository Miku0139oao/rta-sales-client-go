package desktop

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	rtasales "github.com/Miku0139oao/rta-sales-client-go/rtasales"
)

func reportCacheFakeClient() *salesAnalysisFakeClient {
	return &salesAnalysisFakeClient{
		stores: []rtasales.Store{{BusinessID: "107", Label: "107 - First"}},
		results: map[string]*rtasales.SalesResult{
			"107": {Items: []rtasales.SaleItem{{
				PurchaseCategory1Name: "HEALTH", Matnr: "552646", ArticleName: "Mask", BrandName: "Brand",
				TPSaleQuantity: 3, TPSaleAmount: 100, TPGrossSaleQuantity: 3, TPGrossSaleAmount: 100,
			}}},
		},
	}
}

func runCachedAnalysis(t *testing.T, app *App, profileID string) SalesAnalysisResult {
	t.Helper()
	result, err := app.RunSalesAnalysis(SalesAnalysisRequest{
		ProfileID: profileID, StoreIDs: []string{"107"}, From: "2026-08-15", To: "2026-08-15", Concurrency: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	app.reportCache.waitIdle(3 * time.Second)
	return result
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected %s to exist", path)
}

func TestSalesReportSurvivesRestart(t *testing.T) {
	client := reportCacheFakeClient()
	clients := fakeClients{byAccount: map[string]accountClient{"analysis-account": client}}
	app, root, _ := newTestApp(t, new(fakeEngine), clients)
	profile, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		DisplayName: "Analysis", Account: "analysis-account", Password: "password", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	result := runCachedAnalysis(t, app, profile.ID)
	cachePath := filepath.Join(root, salesReportCacheFile)
	waitForFile(t, cachePath)
	info, err := os.Stat(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("cache file should be private, got %v", info.Mode().Perm())
	}

	restarted, _, _ := newTestAppAt(t, root, new(fakeEngine), clients)
	snapshot, err := restarted.LoadSalesAnalysisSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Result == nil || snapshot.Result.OperationID != result.OperationID {
		t.Fatalf("expected restored report %q, got %#v", result.OperationID, snapshot)
	}
	if snapshot.ProfileID != profile.ID || snapshot.SavedAt == "" {
		t.Fatalf("snapshot should carry the account and time: %#v", snapshot)
	}
	if _, err := time.Parse(time.RFC3339, snapshot.SavedAt); err != nil {
		t.Fatalf("savedAt should be RFC3339: %v", err)
	}
	if snapshot.Result.Totals.SaleAmount != 100 || len(snapshot.Result.Periods) != 1 || snapshot.Result.Periods[0].ItemCount != 1 {
		t.Fatalf("restored report lost its summary: %#v", snapshot.Result)
	}
	packed, err := restarted.GetSalesAnalysisItems(SalesAnalysisItemsRequest{OperationID: result.OperationID, PeriodKey: "current"})
	if err != nil {
		t.Fatal(err)
	}
	items := unpackSalesAnalysisItems(packed, snapshot.Result.Stores)
	if len(items) != 1 || items[0].ArticleCode != "552646" || items[0].ArticleName != "Mask" {
		t.Fatalf("restored packed rows should serve item requests: %#v", items)
	}
	glyphs, err := restarted.GetSalesAnalysisReportGlyphs(OperationRequest{OperationID: result.OperationID})
	if err != nil || !bytes.ContainsRune([]byte(glyphs), 'M') {
		t.Fatalf("restored report should feed glyph collection: %q %v", glyphs, err)
	}
}

func TestLoadSalesAnalysisSnapshotIsEmptyWithoutCache(t *testing.T) {
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{byAccount: map[string]accountClient{}})
	snapshot, err := app.LoadSalesAnalysisSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Result != nil || snapshot.ProfileID != "" || snapshot.SavedAt != "" {
		t.Fatalf("expected empty snapshot, got %#v", snapshot)
	}
}

func TestClearSalesAnalysisRemovesReportCache(t *testing.T) {
	client := reportCacheFakeClient()
	clients := fakeClients{byAccount: map[string]accountClient{"analysis-account": client}}
	app, root, _ := newTestApp(t, new(fakeEngine), clients)
	profile, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		DisplayName: "Analysis", Account: "analysis-account", Password: "password", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	result := runCachedAnalysis(t, app, profile.ID)
	cachePath := filepath.Join(root, salesReportCacheFile)
	waitForFile(t, cachePath)
	if err := app.ClearSalesAnalysis(OperationRequest{OperationID: result.OperationID}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cachePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("clear should remove the cache file, stat err=%v", err)
	}
	restarted, _, _ := newTestAppAt(t, root, new(fakeEngine), clients)
	snapshot, err := restarted.LoadSalesAnalysisSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Result != nil {
		t.Fatalf("cleared report should not be restored: %#v", snapshot)
	}
}

func TestLoadSalesAnalysisSnapshotDropsUnusableCache(t *testing.T) {
	writeCache := func(t *testing.T, root string, data []byte) string {
		t.Helper()
		path := filepath.Join(root, salesReportCacheFile)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	gzipJSON := func(t *testing.T, value any) []byte {
		t.Helper()
		var buffer bytes.Buffer
		writer := gzip.NewWriter(&buffer)
		if err := json.NewEncoder(writer).Encode(value); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		return buffer.Bytes()
	}
	cases := map[string][]byte{
		"not gzip": []byte("{not gzip"),
		"wrong version": gzipJSON(t, salesReportDocument{
			Version: 99, SavedAt: time.Now(), Result: SalesAnalysisResult{OperationID: "op"},
		}),
		"pending": gzipJSON(t, salesReportDocument{
			Version: salesReportCacheVersion, SavedAt: time.Now(), Result: SalesAnalysisResult{OperationID: "op", Pending: true},
		}),
		"missing operation": gzipJSON(t, salesReportDocument{
			Version: salesReportCacheVersion, SavedAt: time.Now(), Result: SalesAnalysisResult{},
		}),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			app, root, _ := newTestApp(t, new(fakeEngine), fakeClients{byAccount: map[string]accountClient{}})
			path := writeCache(t, root, data)
			snapshot, err := app.LoadSalesAnalysisSnapshot()
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.Result != nil {
				t.Fatalf("unusable cache should be ignored: %#v", snapshot)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unusable cache should be removed, stat err=%v", err)
			}
		})
	}
}

func TestReportCacheCloseWaitsForPersist(t *testing.T) {
	app, root, _ := newTestApp(t, new(fakeEngine), fakeClients{byAccount: map[string]accountClient{}})
	app.rememberSalesAnalysisFor("profile", SalesAnalysisResult{OperationID: "final-op", Complete: true}, nil)
	app.reportCache.close()
	if _, err := os.Stat(filepath.Join(root, salesReportCacheFile)); err != nil {
		t.Fatalf("close should wait for the cache write, stat err=%v", err)
	}
}

func TestPersistSalesReportSkipsPendingResults(t *testing.T) {
	app, root, _ := newTestApp(t, new(fakeEngine), fakeClients{byAccount: map[string]accountClient{}})
	app.rememberSalesAnalysisFor("profile", SalesAnalysisResult{OperationID: "pending-op", Pending: true}, nil)
	app.reportCache.waitIdle(time.Second)
	if _, err := os.Stat(filepath.Join(root, salesReportCacheFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pending reports must not be persisted, stat err=%v", err)
	}
	app.rememberSalesAnalysisFor("profile", SalesAnalysisResult{OperationID: "final-op", Complete: true}, nil)
	app.reportCache.waitIdle(time.Second)
	waitForFile(t, filepath.Join(root, salesReportCacheFile))
}

func TestScanWorkbookRemembersLastWorkbook(t *testing.T) {
	app, root, _ := newTestApp(t, new(fakeEngine), fakeClients{byAccount: map[string]accountClient{}})
	input := testWorkbook(t)
	scan, err := app.ScanWorkbook(ScanWorkbookRequest{InputPath: input, SheetName: "8月銷售"})
	if err != nil {
		t.Fatal(err)
	}
	sessionPath := filepath.Join(root, workbookSessionFile)
	if _, err := os.Stat(sessionPath); err != nil {
		t.Fatalf("scan should remember the workbook: %v", err)
	}

	restarted, _, _ := newTestAppAt(t, root, new(fakeEngine), fakeClients{byAccount: map[string]accountClient{}})
	session, err := restarted.LoadLastWorkbook()
	if err != nil {
		t.Fatal(err)
	}
	if session.InputPath != scan.InputPath || session.SheetName != scan.SheetName {
		t.Fatalf("unexpected restored session %#v (scan %q/%q)", session, scan.InputPath, scan.SheetName)
	}

	if err := os.Remove(input); err != nil {
		t.Fatal(err)
	}
	session, err = restarted.LoadLastWorkbook()
	if err != nil {
		t.Fatal(err)
	}
	if session.InputPath != "" {
		t.Fatalf("missing workbook should not be restored: %#v", session)
	}
	if _, err := os.Stat(sessionPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale session should be removed, stat err=%v", err)
	}
}

func TestLoadLastWorkbookIsEmptyWithoutSession(t *testing.T) {
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{byAccount: map[string]accountClient{}})
	session, err := app.LoadLastWorkbook()
	if err != nil {
		t.Fatal(err)
	}
	if session.InputPath != "" || session.SheetName != "" {
		t.Fatalf("expected empty session, got %#v", session)
	}
}
