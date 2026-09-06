package desktop

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Miku0139oao/rta-sales-client-go/internal/portableupdate"
)

type installFunc func(context.Context, portableupdate.Candidate, func(string)) (updateReceipt, error)

func (f installFunc) Prepare(ctx context.Context, c portableupdate.Candidate, p func(string)) (updateReceipt, error) {
	return f(ctx, c, p)
}

type fakeReceipt struct {
	commit, cancel func() error
	close          func()
}

func (r *fakeReceipt) Commit() error {
	if r.commit != nil {
		return r.commit()
	}
	return nil
}
func (r *fakeReceipt) Cancel() error {
	if r.cancel != nil {
		return r.cancel()
	}
	return nil
}
func (r *fakeReceipt) Close() {
	if r.close != nil {
		r.close()
	}
}
func installFixture() (a *App, request InstallUpdateRequest) {
	u := newUpdateService()
	u.status.CurrentVersion = "0.4.5"
	u.status.InstallSupported = true
	u.status.UnsupportedReason = ""
	u.status.Phase = "available"
	u.status.CandidateID = "checked"
	u.candidate = &portableupdate.Candidate{} // test-only identity; fake installer never downloads
	u.installer = installFunc(func(context.Context, portableupdate.Candidate, func(string)) (updateReceipt, error) {
		return &fakeReceipt{}, nil
	})
	u.quit = func() {}
	return &App{ctx: context.Background(), updates: u}, InstallUpdateRequest{CandidateID: "checked", Confirmed: true}
}
func TestDevelopmentInstallerCapabilityFailsClosed(t *testing.T) {
	installer, err := nativeUpdateInstaller("dev")
	if err == nil || installer != nil {
		t.Fatal("development installer enabled")
	}
}

func TestInstallRequiresConfirmationFreshCandidateAndCapability(t *testing.T) {
	for _, test := range []string{"unconfirmed", "stale", "unsupported"} {
		t.Run(test, func(t *testing.T) {
			a, request := installFixture()
			called := false
			a.updates.installer = installFunc(func(context.Context, portableupdate.Candidate, func(string)) (updateReceipt, error) {
				called = true
				return &fakeReceipt{}, nil
			})
			switch test {
			case "unconfirmed":
				request.Confirmed = false
			case "stale":
				request.CandidateID = "old"
			case "unsupported":
				a.updates.status.InstallSupported = false
			}
			if err := a.InstallUpdate(request); err == nil || called || a.updateReserved {
				t.Fatal(err, called, a.updateReserved)
			}
		})
	}
}
func TestInstallBusyOwnersRejectReservation(t *testing.T) {
	for _, kind := range []string{"profile-mutation", "profile-test", "sales-supplement", "workbook", "scan-or-dialog-or-write", "export-lease"} {
		t.Run(kind, func(t *testing.T) {
			a, r := installFixture()
			switch kind {
			case "profile-mutation":
				a.profileMutationRunning = true
			case "profile-test":
				a.profileTestRunning = true
			case "sales-supplement":
				a.salesAnalysisRunning = true
			case "workbook":
				a.active = &operationState{running: true}
			case "scan-or-dialog-or-write":
				a.workAdmissions = 1
			case "export-lease":
				a.exportLeases = map[string]struct{}{"pdf": {}}
			}
			if err := a.InstallUpdate(r); err == nil || a.updateReserved {
				t.Fatal("busy install accepted", err)
			}
		})
	}
}
func TestAllPublicWorkEntrypointsRejectUpdateReservation(t *testing.T) {
	a, _ := installFixture()
	a.updateReserved = true
	// All other public methods must fail before touching dependencies or paths.
	exempt := map[string]bool{"CheckRuntime": true, "Cancel": true, "CancelSalesAnalysis": true, "GetUpdateStatus": true, "CheckForUpdate": true, "CheckForUpdateStartup": true, "InstallUpdate": true, "CancelUpdate": true, "EndNativeExportLease": true, "ServiceStartup": true, "ServiceShutdown": true}
	value := reflect.ValueOf(a)
	typ := value.Type()
	for i := 0; i < typ.NumMethod(); i++ {
		method := typ.Method(i)
		if exempt[method.Name] {
			continue
		}
		t.Run(method.Name, func(t *testing.T) {
			fn := value.MethodByName(method.Name)
			args := make([]reflect.Value, fn.Type().NumIn())
			for i := range args {
				args[i] = reflect.Zero(fn.Type().In(i))
			}
			results := fn.Call(args)
			if len(results) == 0 {
				t.Fatal("public work entry has no admission error")
			}
			err, _ := results[len(results)-1].Interface().(error)
			if !errors.Is(err, errUpdateReserved) {
				t.Fatalf("%s bypassed update gate: %v", method.Name, err)
			}
		})
	}
}
func TestInstallFailureReleasesGate(t *testing.T) {
	for _, phase := range []string{"download", "commit"} {
		t.Run(phase, func(t *testing.T) {
			a, r := installFixture()
			cancelled := false
			a.updates.installer = installFunc(func(context.Context, portableupdate.Candidate, func(string)) (updateReceipt, error) {
				if phase == "download" {
					return nil, errors.New("download failed")
				}
				return &fakeReceipt{commit: func() error { return errors.New("commit failed") }, cancel: func() error { cancelled = true; return nil }}, nil
			})
			if err := a.InstallUpdate(r); err == nil || a.updateReserved || a.updates.installing {
				t.Fatal(err, a.updateReserved)
			}
			if phase == "commit" && !cancelled {
				t.Fatal("failed helper not cancelled")
			}
			end, err := a.admitWork()
			if err != nil {
				t.Fatal(err)
			}
			end()
		})
	}
}
func TestInstallCancellationKeepsReservationUntilCleanup(t *testing.T) {
	a, r := installFixture()
	entered := make(chan struct{})
	cleaning := make(chan struct{})
	finishCleanup := make(chan struct{})
	a.updates.installer = installFunc(func(ctx context.Context, _ portableupdate.Candidate, p func(string)) (updateReceipt, error) {
		p("downloading")
		close(entered)
		<-ctx.Done()
		return &fakeReceipt{cancel: func() error { close(cleaning); <-finishCleanup; return nil }}, ctx.Err()
	})
	done := make(chan error, 1)
	go func() { done <- a.InstallUpdate(r) }()
	<-entered
	if err := a.InstallUpdate(r); err == nil {
		t.Fatal("duplicate install accepted")
	}
	if _, err := a.CheckForUpdate(); err == nil {
		t.Fatal("check invalidated installing candidate")
	}
	if err := a.CancelUpdate(); err != nil {
		t.Fatal(err)
	}
	<-cleaning
	if _, err := a.BeginNativeExportLease(); !errors.Is(err, errUpdateReserved) {
		t.Fatal("cleanup released gate too early", err)
	}
	if _, err := a.CheckForUpdate(); err == nil {
		t.Fatal("check during cancelling")
	}
	close(finishCleanup)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if a.updateReserved || a.updates.installing {
		t.Fatal("safe cleanup did not release gate")
	}
}
func TestInstallCommitBoundaryAndUnlockedQuit(t *testing.T) {
	a, r := installFixture()
	committing := make(chan struct{})
	proceed := make(chan struct{})
	var cancelled atomic.Bool
	var quit atomic.Bool
	a.updates.installer = installFunc(func(context.Context, portableupdate.Candidate, func(string)) (updateReceipt, error) {
		return &fakeReceipt{
			commit: func() error { close(committing); <-proceed; return nil }, cancel: func() error { cancelled.Store(true); return nil },
		}, nil
	})
	a.updates.quit = func() {
		a.operationMu.Lock()
		a.operationMu.Unlock()
		a.updates.mu.Lock()
		a.updates.mu.Unlock()
		quit.Store(true)
	}
	done := make(chan error, 1)
	go func() { done <- a.InstallUpdate(r) }()
	<-committing
	if err := a.CancelUpdate(); err == nil {
		t.Fatal("cancellation accepted at commit boundary")
	}
	if _, err := a.admitWork(); !errors.Is(err, errUpdateReserved) {
		t.Fatal(err)
	}
	close(proceed)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if cancelled.Load() || !quit.Load() || !a.updateReserved || !a.shuttingDown {
		t.Fatal("invalid commit ownership")
	}
	if err := a.CancelUpdate(); err == nil {
		t.Fatal("committed helper cancelled")
	}
}
func TestUncertainHelperCleanupNeverReleasesGate(t *testing.T) {
	a, r := installFixture()
	pending := true
	a.updates.installer = installFunc(func(context.Context, portableupdate.Candidate, func(string)) (updateReceipt, error) {
		return &fakeReceipt{
			commit: func() error { return errors.New("commit IPC failed") }, cancel: func() error {
				if pending {
					return errors.New("helper exit pending")
				}
				return nil
			},
		}, nil
	})
	if err := a.InstallUpdate(r); err == nil || !a.updateReserved {
		t.Fatal(err)
	}
	status, _ := a.GetUpdateStatus()
	if status.Phase != "blocked" {
		t.Fatal(status)
	}
	pending = false
	if err := a.CancelUpdate(); err != nil {
		t.Fatal(err)
	}
	if a.updateReserved || a.updates.installing {
		t.Fatal("confirmed helper exit not released")
	}
}
func TestExportLeaseSpansNestedWorkAndIdempotentRelease(t *testing.T) {
	a, r := installFixture()
	id, err := a.BeginNativeExportLease()
	if err != nil {
		t.Fatal(err)
	}
	outer, err := a.admitWork()
	if err != nil {
		t.Fatal(err)
	}
	inner, err := a.admitWork()
	if err != nil {
		t.Fatal(err)
	}
	inner()
	outer()
	if err = a.InstallUpdate(r); err == nil {
		t.Fatal("render gap lost lease")
	}
	if err = a.EndNativeExportLease("unrelated"); err != nil {
		t.Fatal(err)
	}
	if err = a.InstallUpdate(r); err == nil {
		t.Fatal("unrelated release removed lease")
	}
	_ = a.EndNativeExportLease(id)
	_ = a.EndNativeExportLease(id)
	if err = a.reserveUpdate(); err != nil {
		t.Fatal(err)
	}
	if _, err = a.BeginNativeExportLease(); !errors.Is(err, errUpdateReserved) {
		t.Fatal(err)
	}
}
func TestAdmissionReservationRace(t *testing.T) {
	for range 100 {
		a, _ := installFixture()
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		var end func()
		var admissionErr, reserveErr error
		go func() { defer wg.Done(); <-start; end, admissionErr = a.admitWork() }()
		go func() { defer wg.Done(); <-start; reserveErr = a.reserveUpdate() }()
		close(start)
		wg.Wait()
		if admissionErr == nil && reserveErr == nil {
			t.Fatal("work and updater both admitted")
		}
		if end != nil {
			end()
		}
	}
}
