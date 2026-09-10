package desktop

import (
	"strings"
	"time"
)

// persistSalesReport snapshots the in-memory report under salesResultMu and
// writes it in the background. Pending reports are skipped: only the final
// state of a run is worth restoring after a restart.
func (a *App) persistSalesReport() {
	store := a.reportCache
	if store == nil {
		return
	}
	a.salesResultMu.Lock()
	if a.salesResult == nil || a.salesResult.Pending {
		a.salesResultMu.Unlock()
		return
	}
	document := salesReportDocument{
		SavedAt:   a.salesSavedAt,
		ProfileID: a.salesProfileID,
		Result:    *a.salesResult,
		Packed:    make(map[string]SalesAnalysisPackedItems, len(a.salesPacked)),
	}
	// mergeSalesAnalysisSupplement mutates the live map in place; copy the
	// entries so encoding can run outside the lock.
	for key, packed := range a.salesPacked {
		document.Packed[key] = packed
	}
	a.salesResultMu.Unlock()
	if document.SavedAt.IsZero() {
		document.SavedAt = time.Now()
	}
	if !store.beginPersist() {
		return
	}
	go func() {
		defer store.endPersist()
		_ = store.saveSalesReport(document)
	}()
}

func (a *App) clearPersistedSalesReport() {
	if a.reportCache == nil {
		return
	}
	_ = a.reportCache.clearSalesReport()
}

// LoadSalesAnalysisSnapshot returns the last finished report, restoring the
// on-disk cache into memory when the process has just started so item rows,
// memos, and PDF export keep working for the restored report.
func (a *App) LoadSalesAnalysisSnapshot() (SalesAnalysisSnapshot, error) {
	releaseAdmission, admissionErr := a.admitWork()
	if admissionErr != nil {
		return SalesAnalysisSnapshot{}, admissionErr
	}
	defer releaseAdmission()
	if snapshot, ok := a.currentSalesSnapshot(); ok {
		return snapshot, nil
	}
	if a.reportCache == nil {
		return SalesAnalysisSnapshot{}, nil
	}
	document, ok, err := a.reportCache.loadSalesReport()
	if err != nil {
		return SalesAnalysisSnapshot{}, err
	}
	if !ok {
		return SalesAnalysisSnapshot{}, nil
	}
	a.salesResultMu.Lock()
	if a.salesResult == nil {
		result := document.Result
		a.salesResult = &result
		a.salesPacked = document.Packed
		a.salesProfileID = document.ProfileID
		a.salesSavedAt = document.SavedAt
	}
	a.salesResultMu.Unlock()
	snapshot, _ := a.currentSalesSnapshot()
	return snapshot, nil
}

func (a *App) currentSalesSnapshot() (SalesAnalysisSnapshot, bool) {
	a.salesResultMu.Lock()
	defer a.salesResultMu.Unlock()
	if a.salesResult == nil || a.salesResult.Pending {
		return SalesAnalysisSnapshot{}, false
	}
	result := *a.salesResult
	snapshot := SalesAnalysisSnapshot{Result: &result, ProfileID: a.salesProfileID}
	if !a.salesSavedAt.IsZero() {
		snapshot.SavedAt = a.salesSavedAt.Format(time.RFC3339)
	}
	return snapshot, true
}

func (a *App) rememberWorkbookSession(inputPath, sheetName string) {
	if a.reportCache == nil {
		return
	}
	_ = a.reportCache.saveWorkbookSession(inputPath, sheetName)
}

// LoadLastWorkbook names the workbook from the previous session when it still
// exists on disk. The frontend rescans it instead of trusting stale results.
func (a *App) LoadLastWorkbook() (WorkbookSession, error) {
	releaseAdmission, admissionErr := a.admitWork()
	if admissionErr != nil {
		return WorkbookSession{}, admissionErr
	}
	defer releaseAdmission()
	if a.reportCache == nil {
		return WorkbookSession{}, nil
	}
	document, ok, err := a.reportCache.loadWorkbookSession()
	if err != nil {
		return WorkbookSession{}, err
	}
	if !ok {
		return WorkbookSession{}, nil
	}
	inputPath, err := existingWorkbookPath(document.InputPath)
	if err != nil {
		_ = a.reportCache.clearWorkbookSession()
		return WorkbookSession{}, nil
	}
	return WorkbookSession{InputPath: inputPath, SheetName: strings.TrimSpace(document.SheetName)}, nil
}
