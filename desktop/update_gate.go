package desktop

import (
	"errors"
	"sync"
)

var errUpdateReserved = errors.New("update/shutdown is reserved; finish or cancel the update before starting work / 更新或關閉中，無法開始工作")

// Each public work entry holds an admission until it returns. Nested public
// calls acquire independent admissions without holding the mutex across work.
// Background analysis retains its separate salesAnalysisRunning ownership.
func (a *App) admitWork() (func(), error) {
	a.operationMu.Lock()
	if a.updateReserved {
		a.operationMu.Unlock()
		return nil, errUpdateReserved
	}
	a.workAdmissions++
	a.operationMu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { a.operationMu.Lock(); a.workAdmissions--; a.operationMu.Unlock() }) }, nil
}
func (a *App) reserveUpdate() error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	if a.updateReserved {
		return errUpdateReserved
	}
	if a.workAdmissions != 0 || len(a.exportLeases) != 0 || (a.active != nil && a.active.running) || a.profileMutationRunning || a.profileTestRunning || a.salesAnalysisRunning {
		return errors.New("work is still running; finish analysis, account operations and exports before updating / 請先完成分析、帳號操作與匯出")
	}
	a.updateReserved = true
	return nil
}
func (a *App) releaseUpdate() {
	a.operationMu.Lock()
	if !a.shuttingDown {
		a.updateReserved = false
	}
	a.operationMu.Unlock()
}

// BeginNativeExportLease covers frontend rendering and every subsequent write,
// not merely a save dialog or individual RPC. It never expires mid-export.
func (a *App) BeginNativeExportLease() (string, error) {
	if a.updates == nil {
		return "", errUpdatesUnsupported
	}
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	if a.updateReserved {
		return "", errUpdateReserved
	}
	id, err := newUUID()
	if err != nil {
		return "", err
	}
	if a.exportLeases == nil {
		a.exportLeases = make(map[string]struct{})
	}
	a.exportLeases[id] = struct{}{}
	return id, nil
}
func (a *App) EndNativeExportLease(id string) error {
	if a.updates == nil {
		return errUpdatesUnsupported
	}
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	if id == "" {
		return errors.New("empty export lease")
	}
	delete(a.exportLeases, id) // idempotent cleanup; cannot release another lease
	return nil
}

// Stop cancels owned contexts during native shutdown. It does NOT cancel a
// committed helper or clear its reservation. No application callback runs locked.
func Stop(a *App) {
	if a == nil {
		return
	}
	a.operationMu.Lock()
	a.shuttingDown = true
	a.updateReserved = true
	var cancels []func()
	if a.active != nil && a.active.cancel != nil {
		cancels = append(cancels, a.active.cancel)
	}
	if a.profileTestCancel != nil {
		cancels = append(cancels, a.profileTestCancel)
	}
	if a.salesAnalysisCancel != nil {
		cancels = append(cancels, a.salesAnalysisCancel)
	}
	a.operationMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	if u := a.updates; u != nil {
		u.mu.Lock()
		if u.status.Phase != "committing" && u.status.Phase != "committed" && u.cancel != nil {
			u.cancel()
		}
		u.mu.Unlock()
	}
}
