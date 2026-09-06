package desktop

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Miku0139oao/rta-sales-client-go/internal/portableupdate"
)

type checkFunc func(context.Context, string) (portableupdate.Inspection, error)

func (f checkFunc) Inspect(ctx context.Context, v string) (portableupdate.Inspection, error) {
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
	u.client = checkFunc(func(ctx context.Context, _ string) (portableupdate.Inspection, error) {
		close(entered)
		<-ctx.Done()
		return portableupdate.Inspection{}, ctx.Err()
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
func TestCurrentAndOlderChangelogNeverBecomeInstallCandidates(t *testing.T) {
	for _, latest := range []string{"0.4.7", "0.4.6"} {
		t.Run(latest, func(t *testing.T) {
			u := newUpdateService()
			u.status.CurrentVersion = "0.4.7"
			calls := 0
			u.client = checkFunc(func(context.Context, string) (portableupdate.Inspection, error) {
				calls++
				return portableupdate.Inspection{Version: latest, Body: "<script>escaped notes</script>"}, nil
			})
			a := &App{ctx: context.Background(), updates: u}
			s, err := a.CheckForUpdate()
			if err != nil || s.Phase != "current" || s.ChangelogVersion != latest || s.ChangelogBody != "<script>escaped notes</script>" || calls != 1 {
				t.Fatal(s, err, calls)
			}
			if s.CandidateID != "" || s.AvailableVersion != "" || s.ReleaseNotes != "" || u.candidate != nil {
				t.Fatal("fabricated install candidate", s)
			}
			if err := a.InstallUpdate(InstallUpdateRequest{Confirmed: true}); err == nil {
				t.Fatal("installed metadata-only release")
			}
			persisted, _ := a.GetUpdateStatus()
			if persisted != s {
				t.Fatal("metadata not retained")
			}
		})
	}
}

func TestMetadataChecksCoexistWithWork(t *testing.T) {
	u := newUpdateService()
	u.client = checkFunc(func(context.Context, string) (portableupdate.Inspection, error) {
		return portableupdate.Inspection{}, nil
	})
	a := &App{ctx: context.Background(), updates: u, salesAnalysisRunning: true, profileMutationRunning: true}
	s, err := a.CheckForUpdate()
	if err != nil || s.Phase != "current" {
		t.Fatal(s, err)
	}
	if !a.salesAnalysisRunning || !a.profileMutationRunning {
		t.Fatal("metadata check altered work ownership")
	}
}
