package desktop

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Miku0139oao/rta-sales-client-go/internal/portableupdate"
)

type checkFunc func(context.Context, string) (*portableupdate.Candidate, error)

func (f checkFunc) Check(ctx context.Context, v string) (*portableupdate.Candidate, error) {
	return f(ctx, v)
}
func TestWebRPCRejectsUpdatesEvenWithNativeService(t *testing.T) {
	session := &webSession{app: &App{updates: newUpdateService()}}
	for _, method := range []string{"GetUpdateStatus", "CheckForUpdate", "InstallUpdate", "CancelUpdate", "BeginNativeExportLease", "EndNativeExportLease"} {
		if _, err := dispatchWebRPC(session, method, nil); err == nil {
			t.Fatalf("web exposed %s", method)
		}
	}
}

func TestUpdateAPIsUnsupportedWithoutNativeWiring(t *testing.T) {
	a := &App{}
	if _, err := a.GetUpdateStatus(); !errors.Is(err, errUpdatesUnsupported) {
		t.Fatal(err)
	}
	if _, err := a.CheckForUpdate(); !errors.Is(err, errUpdatesUnsupported) {
		t.Fatal(err)
	}
	if err := a.CancelUpdate(); !errors.Is(err, errUpdatesUnsupported) {
		t.Fatal(err)
	}
	if err := a.InstallUpdate(InstallUpdateRequest{CandidateID: "candidate", Confirmed: true}); !errors.Is(err, errUpdatesUnsupported) {
		t.Fatal(err)
	}
}
func TestUpdateCheckCancellationAndConcurrency(t *testing.T) {
	entered := make(chan struct{})
	u := newUpdateService()
	u.client = checkFunc(func(ctx context.Context, _ string) (*portableupdate.Candidate, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	a := &App{ctx: context.Background(), updates: u}
	done := make(chan error, 1)
	go func() { _, err := a.CheckForUpdate(); done <- err }()
	<-entered
	if _, err := a.CheckForUpdate(); err == nil {
		t.Fatal("overlapping check accepted")
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := a.GetUpdateStatus(); err != nil {
				t.Error(err)
			}
		}()
	}
	if err := a.CancelUpdate(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	wg.Wait()
	s, _ := a.GetUpdateStatus()
	if s.Phase != "idle" || s.CandidateID != "" || s.InstallSupported {
		t.Fatal(s)
	}
	if err := a.InstallUpdate(InstallUpdateRequest{CandidateID: "stale", Confirmed: true}); err == nil {
		t.Fatal("install unexpectedly enabled")
	}
}
func TestMetadataChecksCoexistWithWork(t *testing.T) {
	u := newUpdateService()
	u.client = checkFunc(func(context.Context, string) (*portableupdate.Candidate, error) { return nil, nil })
	a := &App{ctx: context.Background(), updates: u, salesAnalysisRunning: true, profileMutationRunning: true}
	s, err := a.CheckForUpdate()
	if err != nil || s.Phase != "current" {
		t.Fatal(s, err)
	}
	if !a.salesAnalysisRunning || !a.profileMutationRunning {
		t.Fatal("metadata check altered work ownership")
	}
}
