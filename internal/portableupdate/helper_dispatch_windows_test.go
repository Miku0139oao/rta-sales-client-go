//go:build windows

package portableupdate

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The copied package test executable is a dedicated sandbox helper, not the
// desktop application. Only this test entry point adds a harmless parent mode.
func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "--wait" {
		done := make(chan struct{})
		go func() { _, _ = io.Copy(io.Discard, os.Stdin); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
		}
		os.Exit(0)
	}
	if handled, code := DispatchHelper(os.Args[1:]); handled {
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func TestWindowsCancelledPipeStillRequiresHelperExit(t *testing.T) {
	done := make(chan struct{})
	session := &HelperSession{done: done} // pipe was closed by a previous cancellation
	result := make(chan error, 1)
	go func() { result <- session.Cancel() }()
	select {
	case err := <-result:
		t.Fatalf("closed input incorrectly proved helper exit: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(done)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if err := session.Cancel(); err != nil {
		t.Fatal("confirmed exit must be idempotent", err)
	}
}

func TestWindowsActualHelperDispatchRejectsUnsigned(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := sandbox(t, data)
	// Parent-side test injection permits starting ONLY the copied unsigned test
	// helper. Inside that new process DispatchHelper always uses the real Windows
	// verifier: there is no environment/config switch to bypass trust.
	if _, err = startHelperChecked(context.Background(), p.config, sandboxVerifier{}); err == nil {
		t.Fatal("unsigned helper unexpectedly became ready")
	}
	raw, err := os.ReadFile(filepath.Join(p.config.Directory, "result.json"))
	if err != nil {
		t.Fatal("actual helper failed to retain rejection diagnostic", err)
	}
	var result Result
	if err = json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.Phase != "prepare" || !strings.Contains(result.Error, "Authenticode") {
		t.Fatal("actual helper did not fail trust", result)
	}
	if _, err = os.Stat(p.backupPath()); !os.IsNotExist(err) {
		t.Fatal("unsigned helper moved target")
	}
}
