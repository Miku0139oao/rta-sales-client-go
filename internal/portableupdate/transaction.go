package portableupdate

import (
	"context"
	"errors"
	"fmt"
)

// TransactionOS describes the helper contract. The Windows adapter implements
// it with sandbox-tested handle, ACL, reparse and process-launch semantics.
// Desktop installation reserves its work gate before invoking the adapter.
// UI paths can never directly construct a production transaction.
//
// Prepare must acquire a real parent process handle, validate all private
// same-volume paths/file identities and reserve a unique backup. Ready signals
// readiness through authenticated private IPC, before the parent can quit.
// WaitParent never terminates the parent. Verify locks and revalidates the
// candidate and trusted current image immediately before replacement.
// MoveOld, MoveNew and RestoreOld MUST fail atomically without altering either
// file on error. Restart launches only the exact original target and cwd, and
// must not return an error while a child could still be using that executable.
// Backups, staging and diagnostic results are retained on every failure.
type TransactionOS interface {
	Prepare(context.Context) error
	Ready(context.Context) error
	WaitParent(context.Context) error
	Verify(context.Context) error
	MoveOld() error
	MoveNew() error
	Restart() error
	RestoreOld() error
	Record(Result) error
}

type Result struct {
	Phase      string `json:"phase"`
	Error      string `json:"error,omitempty"`
	RolledBack bool   `json:"rolledBack"`
}

// RunTransaction is the context-aware orchestration core used by the Windows
// helper. Context deadlines cannot forcibly interrupt synchronous OS APIs.
func RunTransaction(ctx context.Context, os TransactionOS) (result Result, err error) {
	if os == nil {
		return Result{Phase: "prepare"}, errors.New("helper platform unavailable")
	}
	defer func() {
		if err != nil {
			result.Error = err.Error()
		}
		if recordErr := os.Record(result); recordErr != nil {
			err = errors.Join(err, fmt.Errorf("retain diagnostic: %w", recordErr))
		}
	}()
	steps := []struct {
		name string
		run  func(context.Context) error
	}{
		{"prepare", os.Prepare}, {"ready", os.Ready}, {"wait-parent", os.WaitParent}, {"verify", os.Verify},
	}
	for _, step := range steps {
		result.Phase = step.name
		if err = ctx.Err(); err != nil {
			return
		}
		if err = step.run(ctx); err != nil {
			return
		}
	}
	result.Phase = "backup"
	if err = ctx.Err(); err != nil {
		return
	}
	if err = os.MoveOld(); err != nil {
		return
	}
	// Once the good executable has moved, cancellation must not interrupt recovery.
	result.Phase = "replace"
	if err = os.MoveNew(); err != nil {
		return rollback(os, result, err)
	}
	result.Phase = "restart"
	if err = os.Restart(); err != nil {
		return rollback(os, result, err)
	}
	result.Phase = "complete"
	return
}

func rollback(os TransactionOS, result Result, cause error) (Result, error) {
	if err := os.RestoreOld(); err != nil {
		return result, errors.Join(cause, fmt.Errorf("rollback failed; retain backup for manual recovery: %w", err))
	}
	result.RolledBack = true
	if err := os.Restart(); err != nil {
		return result, errors.Join(cause, fmt.Errorf("old executable restored but restart failed: %w", err))
	}
	return result, cause
}
