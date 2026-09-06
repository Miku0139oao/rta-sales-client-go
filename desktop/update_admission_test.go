package desktop

import (
	"context"
	"testing"
)

type admissionDialogs struct {
	dialogService
	entered, release chan struct{}
}

func (d admissionDialogs) OpenDirectory(context.Context, fileDialogOptions) (string, error) {
	close(d.entered)
	<-d.release
	return "", nil
}

type admissionEngine struct {
	fakeEngine
	entered, release chan struct{}
}

func (e *admissionEngine) Scan(engineScanRequest) (WorkbookScan, error) {
	close(e.entered)
	<-e.release
	return WorkbookScan{}, nil
}

type admissionCatalog struct {
	manCodeRepository
	entered, release chan struct{}
}

func (c admissionCatalog) List() ([]ManCodeGroup, error) {
	close(c.entered)
	<-c.release
	return nil, nil
}
func TestRealAdmissionLifetimesBlockInstall(t *testing.T) {
	for _, kind := range []string{"directory-dialog", "workbook-scan", "catalog-read"} {
		t.Run(kind, func(t *testing.T) {
			a, _, _ := newTestApp(t, &fakeEngine{}, fakeClients{})
			fixture, request := installFixture()
			a.updates = fixture.updates
			entered, release := make(chan struct{}), make(chan struct{})
			var call func() error
			switch kind {
			case "directory-dialog":
				a.dialogs = admissionDialogs{a.dialogs, entered, release}
				call = func() error { _, err := a.ChooseSalesAnalysisPDFDirectory(); return err }
			case "workbook-scan":
				a.engine = &admissionEngine{entered: entered, release: release}
				path := testWorkbook(t)
				call = func() error { _, err := a.ScanWorkbook(ScanWorkbookRequest{InputPath: path}); return err }
			case "catalog-read":
				a.mancodes = admissionCatalog{a.mancodes, entered, release}
				call = func() error { _, err := a.ListManCodeGroups(); return err }
			}
			done := make(chan error, 1)
			go func() { done <- call() }()
			<-entered
			if err := a.InstallUpdate(request); err == nil {
				t.Fatal("install admitted during " + kind)
			}
			close(release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if err := a.reserveUpdate(); err != nil {
				t.Fatal("finished admission leaked", err)
			}
		})
	}
}
func TestSupplementOwnershipOutlivesFrontendCallAdmission(t *testing.T) {
	a, r := installFixture()
	end, err := a.admitWork()
	if err != nil {
		t.Fatal(err)
	}
	_, finish, err := a.beginSalesAnalysisOperation("supplement")
	if err != nil {
		t.Fatal(err)
	}
	end() // the RPC has returned its primary result, but supplements still run
	if err = a.InstallUpdate(r); err == nil {
		t.Fatal("supplement ownership lost after primary result")
	}
	finish()
	if err = a.reserveUpdate(); err != nil {
		t.Fatal(err)
	}
}
