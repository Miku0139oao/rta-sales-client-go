package desktop

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"time"

	"github.com/Miku0139oao/rta-sales-client-go/internal/buildinfo"
	"github.com/Miku0139oao/rta-sales-client-go/internal/portableupdate"
)

var errUpdatesUnsupported = errors.New("portable updates are available only in the Windows native app / 僅 Windows 原生程式支援更新")

type UpdateStatus struct {
	CurrentVersion    string `json:"currentVersion"`
	Phase             string `json:"phase"`
	CandidateID       string `json:"candidateId"`
	AvailableVersion  string `json:"availableVersion"`
	ReleaseNotes      string `json:"releaseNotes"`
	InstallSupported  bool   `json:"installSupported"`
	UnsupportedReason string `json:"unsupportedReason"`
	Error             string `json:"error"`
}
type InstallUpdateRequest struct {
	CandidateID string `json:"candidateId"`
	Confirmed   bool   `json:"confirmed"`
}
type updateChecker interface {
	Check(context.Context, string) (*portableupdate.Candidate, error)
}
type updateReceipt interface {
	Commit() error
	Cancel() error
	Close()
}
type updateInstaller interface {
	Prepare(context.Context, portableupdate.Candidate, func(string)) (updateReceipt, error)
}
type updateService struct {
	mu         sync.Mutex
	client     updateChecker
	status     UpdateStatus
	candidate  *portableupdate.Candidate
	cancel     context.CancelFunc
	generation uint64
	installing bool
	installer  updateInstaller
	quit       func()
	receipt    updateReceipt
}

func newUpdateService() *updateService {
	return &updateService{client: portableupdate.NewClient(), status: UpdateStatus{CurrentVersion: buildinfo.Version, Phase: "idle", UnsupportedReason: "Native update lifecycle is not configured / 原生更新生命週期尚未設定"}}
}
func (u *updateService) reset(phase string) {
	u.status = UpdateStatus{CurrentVersion: u.status.CurrentVersion, Phase: phase, InstallSupported: u.status.InstallSupported, UnsupportedReason: u.status.UnsupportedReason}
}
func (a *App) GetUpdateStatus() (UpdateStatus, error) {
	if a.updates == nil {
		return UpdateStatus{}, errUpdatesUnsupported
	}
	a.updates.mu.Lock()
	defer a.updates.mu.Unlock()
	return a.updates.status, nil
}
func (a *App) CheckForUpdate() (UpdateStatus, error) {
	u := a.updates
	if u == nil {
		return UpdateStatus{}, errUpdatesUnsupported
	}
	u.mu.Lock()
	if u.cancel != nil || u.installing {
		status := u.status
		u.mu.Unlock()
		return status, errors.New("update operation already running / 更新作業進行中")
	}
	ctx, cancel := context.WithTimeout(a.appContext(), 30*time.Second)
	u.cancel = cancel
	u.generation++
	generation := u.generation
	u.reset("checking")
	u.candidate = nil
	version := u.status.CurrentVersion
	u.mu.Unlock()
	candidate, err := u.client.Check(ctx, version)
	cancel()
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.generation != generation {
		return u.status, context.Canceled
	}
	u.cancel = nil
	if err != nil {
		u.status.Phase = "error"
		u.status.Error = err.Error()
		return u.status, err
	}
	u.status.Phase = "current"
	if candidate != nil {
		id, idErr := newUUID()
		if idErr != nil {
			u.status.Phase = "error"
			u.status.Error = idErr.Error()
			return u.status, idErr
		}
		u.candidate = candidate
		u.status.Phase = "available"
		u.status.CandidateID = id
		u.status.AvailableVersion = candidate.Version()
		u.status.ReleaseNotes = candidate.Notes()
	}
	return u.status, nil
}
func (a *App) CancelUpdate() error {
	u := a.updates
	if u == nil {
		return errUpdatesUnsupported
	}
	u.mu.Lock()
	if u.status.Phase == "committing" || u.status.Phase == "committed" {
		u.mu.Unlock()
		return errors.New("shutdown already committed; cancellation is no longer safe / 已確認關閉，無法取消")
	}
	if u.installing {
		if u.cancel != nil {
			u.cancel()
		}
		if u.status.Phase == "blocked" && u.receipt != nil {
			receipt := u.receipt
			u.status.Phase = "cancelling"
			u.mu.Unlock()
			err := receipt.Cancel()
			u.mu.Lock()
			defer u.mu.Unlock()
			if err != nil {
				u.status.Phase = "blocked"
				u.status.Error = err.Error()
				return err
			}
			receipt.Close()
			u.receipt = nil
			u.cancel = nil
			u.installing = false
			u.reset("idle")
			u.candidate = nil
			a.releaseUpdate()
			return nil
		}
		u.status.Phase = "cancelling"
		u.mu.Unlock()
		return nil // worker releases only after cleanup
	}
	if u.cancel != nil {
		u.cancel()
		u.cancel = nil
	}
	u.generation++
	u.candidate = nil
	u.reset("idle")
	u.mu.Unlock()
	return nil
}

// InstallUpdate accepts only a checked backend candidate and explicit consent.
// Lock ordering is updateService.mu -> operationMu. Work admission never takes
// updateService.mu. No installer, IPC or application quit runs under either lock.
func (a *App) InstallUpdate(request InstallUpdateRequest) (err error) {
	u := a.updates
	if u == nil {
		return errUpdatesUnsupported
	}
	u.mu.Lock()
	if !request.Confirmed {
		u.mu.Unlock()
		return errors.New("explicit update confirmation is required / 必須明確確認更新")
	}
	if u.installing || u.cancel != nil {
		u.mu.Unlock()
		return errors.New("update operation already running / 更新作業進行中")
	}
	if request.CandidateID == "" || request.CandidateID != u.status.CandidateID || u.candidate == nil {
		u.mu.Unlock()
		return errors.New("update candidate is no longer valid / 更新候選已失效")
	}
	if !u.status.InstallSupported || u.installer == nil || u.quit == nil {
		reason := u.status.UnsupportedReason
		u.mu.Unlock()
		return errors.New("installation unsupported: " + reason)
	}
	if err = a.reserveUpdate(); err != nil {
		u.mu.Unlock()
		return err
	}
	candidate := *u.candidate
	installer, quit := u.installer, u.quit
	ctx, cancel := context.WithCancel(a.appContext())
	u.cancel = cancel
	u.installing = true
	u.status.Phase = "preparing"
	u.status.Error = ""
	u.mu.Unlock()
	var receipt updateReceipt
	committed := false
	defer func() {
		cancel()
		if committed {
			if receipt != nil {
				receipt.Close()
			}
			return
		}
		var cleanupErr error
		if receipt != nil {
			cleanupErr = receipt.Cancel()
		}
		u.mu.Lock()
		defer u.mu.Unlock()
		if cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
			u.receipt = receipt
			u.status.Phase = "blocked"
			u.status.Error = "Helper cancellation not yet confirmed; keep this app open and retry cancel / 請保持程式開啟並重試取消: " + err.Error()
			return // uncertain helper ownership: NEVER release the work gate
		}
		if receipt != nil {
			receipt.Close()
		}
		u.receipt = nil
		u.cancel = nil
		u.installing = false
		if errors.Is(err, context.Canceled) {
			u.reset("idle")
			u.candidate = nil
		} else {
			u.status.Phase = "error"
			if err != nil {
				u.status.Error = err.Error()
			}
		}
		a.releaseUpdate()
	}()
	receipt, err = installer.Prepare(ctx, candidate, func(phase string) {
		u.mu.Lock()
		defer u.mu.Unlock()
		if u.status.Phase != "cancelling" {
			u.status.Phase = phase
		}
	})
	if err != nil {
		return err
	}
	if receipt == nil {
		return errors.New("installer returned no verified helper receipt")
	}
	u.mu.Lock()
	if err = ctx.Err(); err != nil {
		u.mu.Unlock()
		return err
	}
	u.receipt = receipt
	u.status.Phase = "committing" // cancellation boundary, atomically
	u.mu.Unlock()
	if err = receipt.Commit(); err != nil {
		return err
	}
	u.mu.Lock()
	u.status.Phase = "committed"
	u.cancel = nil
	u.mu.Unlock()
	committed = true
	Stop(a) // cancel application-owned contexts, preserving committed helper
	quit()  // deliberately outside ALL mutexes
	return nil
}

// ConfigureNativeUpdateLifecycle is a Go-only integration hook, not a bound App
// method. Call once before app.Run. Unsupported/dev/untrusted builds stay honest.
func ConfigureNativeUpdateLifecycle(a *App, quit func()) {
	if a == nil || a.updates == nil {
		return
	}
	installer, err := nativeUpdateInstaller(buildinfo.Version)
	u := a.updates
	u.mu.Lock()
	defer u.mu.Unlock()
	if err != nil {
		u.status.UnsupportedReason = err.Error()
		return
	}
	if quit == nil {
		u.status.UnsupportedReason = "Native quit callback missing / 缺少原生退出回呼"
		return
	}
	u.installer = installer
	u.quit = quit
	u.status.InstallSupported = true
	u.status.UnsupportedReason = ""
}
func enableNativeUpdates(a *App) {
	if runtime.GOOS == "windows" {
		a.updates = newUpdateService()
	}
}
