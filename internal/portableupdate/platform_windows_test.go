//go:build windows

package portableupdate

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// This dedicated dummy has no application code, credentials, UI, network or
// update/replacement behavior. Parent mode exits on pipe close or its own safety
// deadline. No test ever terminates a process or launches the real application.
const sandboxProgram = `package main
import("bufio";"encoding/json";"fmt";"io";"os";"time")
func main(){
 if len(os.Args)>1&&os.Args[1]=="--wait" {done:=make(chan struct{});go func(){io.Copy(io.Discard,os.Stdin);close(done)}();select{case<-done:case<-time.After(10*time.Second):};return}
 if len(os.Args)>1&&os.Args[1]=="--portable-update-helper" {
  r:=bufio.NewReader(os.Stdin);line,_:=r.ReadBytes('\n');var c struct{Token string};json.Unmarshal(line,&c)
  if os.Getenv("RTA_SANDBOX_BAD_READY")=="1" {fmt.Println("ready:wrong");return}
  if os.Getenv("RTA_SANDBOX_NO_READY")=="1" {io.Copy(io.Discard,r);return}
  fmt.Println("ready:"+c.Token);line,_=r.ReadBytes('\n');if string(line)=="commit:"+c.Token+"\n" {fmt.Println("commit-accepted:"+c.Token);r.ReadString('\n')};return
 }
 os.WriteFile("sandbox-restarted.txt",[]byte("dummy only"),0600)
}
`

type sandboxVerifier struct{}

func (sandboxVerifier) VerifyIdentity(path string) (Identity, error) {
	version := "0.4.5"
	data, _ := os.ReadFile(path)
	if filepath.Base(path) == "candidate.exe" || bytes.HasSuffix(data, []byte("candidate-marker")) {
		version = "0.5.0"
	}
	return Identity{true, "CN=Sandbox injected policy (NOT an actual signature)", version}, nil
}

// The machine's TEMP can intentionally grant a separate sandbox principal
// Modify access. Never weaken the production policy to accommodate that:
// execute test copies only below a fresh private directory in the user cache.
func safeSandboxRoot(t *testing.T) string {
	t.Helper()
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := privateDirectory(cache)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		deadline := time.Now().Add(5 * time.Second)
		for {
			err := os.RemoveAll(dir)
			if err == nil {
				return
			}
			if time.Now().After(deadline) {
				t.Errorf("sandbox cleanup: %v", err)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
	return dir
}
func buildSandboxDummy(t *testing.T) []byte {
	t.Helper()
	dir := safeSandboxRoot(t)
	source := filepath.Join(dir, "dummy.go")
	exe := filepath.Join(dir, "dummy.exe")
	if err := os.WriteFile(source, []byte(sandboxProgram), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-trimpath", "-o", exe, source)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build dedicated sandbox dummy: %v %s", err, output)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func sandbox(t *testing.T, dummy []byte) (*windowsTransaction, func()) {
	t.Helper()
	dir := filepath.Join(safeSandboxRoot(t), "銷售 測試 Ω")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "銷售 portable.exe")
	if err := os.WriteFile(target, dummy, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := newStaging(target, dir, sandboxVerifier{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	candidate := filepath.Join(s.config.Directory, "candidate.exe")
	next := append(append([]byte{}, dummy...), []byte("candidate-marker")...)
	if err = os.WriteFile(candidate, next, 0600); err != nil {
		t.Fatal(err)
	}
	f, err := openLocked(candidate, false)
	if err != nil {
		t.Fatal(err)
	}
	s.config.CandidateID, err = idOf(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	s.config.Hash = sha256.Sum256(next)
	s.config.Version = "0.5.0"
	cmd := exec.Command(target, "--wait")
	cmd.Dir = dir
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	s.config.ParentPID = uint32(cmd.Process.Pid)
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, s.config.ParentPID)
	if err != nil {
		t.Fatal(err)
	}
	s.config.ParentCreated, err = processCreated(h)
	windows.CloseHandle(h)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	release := func() { _ = stdin.Close() }
	t.Cleanup(func() {
		release()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("dummy parent: %v", err)
			}
		case <-time.After(12 * time.Second):
			t.Error("dummy failed its own safety deadline; no force termination attempted")
		}
	})
	p := newWindowsTransaction(s.config)
	p.self = filepath.Join(s.config.Directory, "helper.exe")
	p.verifier = sandboxVerifier{}
	p.ready = func(context.Context) error { release(); return nil }
	t.Cleanup(p.Close)
	return p, release
}
func waitMarker(t *testing.T, dir string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(dir, "sandbox-restarted.txt")); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("dedicated dummy restart did not produce cwd marker")
}
func TestWindowsSandboxTransactions(t *testing.T) {
	dummy := buildSandboxDummy(t)
	t.Run("unicode actual wait rename restart backup and record", func(t *testing.T) {
		p, _ := sandbox(t, dummy)
		result, err := RunTransaction(context.Background(), p)
		p.Close()
		if err != nil || result.Phase != "complete" {
			t.Fatal(result, err)
		}
		waitMarker(t, p.config.CWD)
		old, err := os.ReadFile(p.backupPath())
		if err != nil || !bytes.Equal(old, dummy) {
			t.Fatal("good backup not retained", err)
		}
		next, err := os.ReadFile(p.config.Target)
		if err != nil || sha256.Sum256(next) != p.config.Hash {
			t.Fatal("wrong target", err)
		}
		raw, err := os.ReadFile(filepath.Join(p.config.Directory, "result.json"))
		if err != nil {
			t.Fatal(err)
		}
		var record Result
		if json.Unmarshal(raw, &record) != nil || record.Phase != "complete" {
			t.Fatal("missing retained result")
		}
	})
	t.Run("parent handle timeout never replaces", func(t *testing.T) {
		p, _ := sandbox(t, dummy)
		// Start the deadline after preparation: race instrumentation/AV scanning
		// must not accidentally turn a parent-wait test into a prepare timeout.
		if err := p.Prepare(context.Background()); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		err := p.WaitParent(ctx)
		p.Close()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
		original, _ := os.ReadFile(p.config.Target)
		if !bytes.Equal(original, dummy) {
			t.Fatal("timeout modified target")
		}
	})
	t.Run("readiness failure never replaces", func(t *testing.T) {
		p, _ := sandbox(t, dummy)
		p.ready = func(context.Context) error { return errors.New("private IPC closed") }
		result, err := RunTransaction(context.Background(), p)
		if err == nil || result.Phase != "ready" {
			t.Fatal(result, err)
		}
		if _, err = os.Stat(p.backupPath()); !os.IsNotExist(err) {
			t.Fatal("backup moved before readiness")
		}
	})
	t.Run("tamper after ready never launches", func(t *testing.T) {
		p, release := sandbox(t, dummy)
		launched := false
		p.launch = func(string, string) error { launched = true; return nil }
		p.ready = func(context.Context) error {
			release()
			return os.WriteFile(p.candidatePath(), []byte("tampered"), 0600)
		}
		result, err := RunTransaction(context.Background(), p)
		if err == nil || result.Phase != "verify" || launched {
			t.Fatal(result, err, launched)
		}
	})
	t.Run("file lock blocks rename-handle acquisition", func(t *testing.T) {
		p, _ := sandbox(t, dummy)
		locked, err := openLocked(p.candidatePath(), false)
		if err != nil {
			t.Fatal(err)
		}
		defer locked.Close()
		result, err := RunTransaction(context.Background(), p)
		if err == nil || result.Phase != "verify" {
			t.Fatal(result, err)
		}
		if _, err = os.Stat(p.backupPath()); !os.IsNotExist(err) {
			t.Fatal("moved old despite lock")
		}
	})
	t.Run("injected replacement failure rolls back and retains backup", func(t *testing.T) {
		p, _ := sandbox(t, dummy)
		p.rename = func(f *os.File, dst string, replace bool) error {
			if f == p.next {
				return errors.New("injected MoveNew failure")
			}
			return renameHandle(f, dst, replace)
		}
		result, err := RunTransaction(context.Background(), p)
		p.Close()
		if err == nil || !result.RolledBack {
			t.Fatal(result, err)
		}
		waitMarker(t, p.config.CWD)
		target, _ := os.ReadFile(p.config.Target)
		backup, _ := os.ReadFile(p.backupPath())
		if !bytes.Equal(target, dummy) || !bytes.Equal(backup, dummy) {
			t.Fatal("rollback consumed or changed good backup")
		}
	})
	t.Run("CreateProcess failure rolls back and retains backup", func(t *testing.T) {
		p, _ := sandbox(t, dummy)
		calls := 0
		p.launch = func(target, cwd string) error {
			calls++
			if calls == 1 {
				return restartExact(target, filepath.Join(cwd, "nonexistent-cwd"))
			}
			return restartExact(target, cwd)
		}
		result, err := RunTransaction(context.Background(), p)
		p.Close()
		if err == nil || !result.RolledBack || calls != 2 {
			t.Fatal(result, err, calls)
		}
		waitMarker(t, p.config.CWD)
		backup, _ := os.ReadFile(p.backupPath())
		if !bytes.Equal(backup, dummy) {
			t.Fatal("backup consumed")
		}
	})
	t.Run("rollback failure retains sole good backup", func(t *testing.T) {
		p, _ := sandbox(t, dummy)
		p.launch = func(string, string) error { return errors.New("injected restart failure") }
		p.rename = func(f *os.File, dst string, replace bool) error {
			if replace {
				return errors.New("injected rollback failure")
			}
			return renameHandle(f, dst, replace)
		}
		result, err := RunTransaction(context.Background(), p)
		p.Close()
		if err == nil || result.RolledBack {
			t.Fatal(result, err)
		}
		backup, _ := os.ReadFile(p.backupPath())
		if !bytes.Equal(backup, dummy) {
			t.Fatal("lost sole good backup")
		}
	})
	t.Run("real rename collision preserves unrelated target and backup", func(t *testing.T) {
		p, _ := sandbox(t, dummy)
		p.rename = func(f *os.File, dst string, replace bool) error {
			err := renameHandle(f, dst, replace)
			if err == nil && f == p.old {
				return os.WriteFile(p.config.Target, []byte("unrelated recreated file"), 0600)
			}
			return err
		}
		result, err := RunTransaction(context.Background(), p)
		p.Close()
		if err == nil || result.RolledBack {
			t.Fatal(result, err)
		}
		backup, _ := os.ReadFile(p.backupPath())
		target, _ := os.ReadFile(p.config.Target)
		if !bytes.Equal(backup, dummy) || string(target) != "unrelated recreated file" {
			t.Fatal("collision lost data")
		}
	})
	t.Run("wrong process creation time rejected", func(t *testing.T) {
		p, _ := sandbox(t, dummy)
		p.config.ParentCreated.LowDateTime++
		if err := p.Prepare(context.Background()); err == nil {
			t.Fatal("PID reuse identity accepted")
		}
	})
	t.Run("wrong publisher rejected before readiness", func(t *testing.T) {
		p, _ := sandbox(t, dummy)
		p.verifier = fakeVerifier{
			p.config.Target:   {Trusted: true, PublisherSubject: "CN=A", Version: "0.4.5"},
			p.candidatePath(): {Trusted: true, PublisherSubject: "CN=B", Version: "0.5.0"},
		}
		if err := p.Prepare(context.Background()); err == nil {
			t.Fatal("wrong publisher accepted")
		}
	})
	t.Run("helper tamper refused before CreateProcess", func(t *testing.T) {
		p, _ := sandbox(t, dummy)
		if err := os.WriteFile(p.self, []byte("tampered helper"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := startHelperChecked(context.Background(), p.config, sandboxVerifier{}); err == nil {
			t.Fatal("tampered helper launched")
		}
	})
	t.Run("bad private readiness token rejected", func(t *testing.T) {
		t.Setenv("RTA_SANDBOX_BAD_READY", "1")
		p, _ := sandbox(t, dummy)
		if _, err := startHelperChecked(context.Background(), p.config, sandboxVerifier{}); err == nil {
			t.Fatal("wrong token accepted")
		}
	})
	t.Run("actual helper readiness timeout", func(t *testing.T) {
		t.Setenv("RTA_SANDBOX_NO_READY", "1")
		p, _ := sandbox(t, dummy)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		if _, err := startHelperChecked(ctx, p.config, sandboxVerifier{}); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
	})
	t.Run("actual inherited pipe readiness transport", func(t *testing.T) {
		p, _ := sandbox(t, dummy)
		// This transport fixture only acknowledges a frame; it never replaces files.
		// Actual transaction filesystem/process behavior is tested above, separately.
		session, err := startHelperChecked(context.Background(), p.config, sandboxVerifier{})
		if err != nil {
			t.Fatal(err)
		}
		if err = session.Commit(); err != nil {
			t.Fatal(err)
		}
		if err = session.Cancel(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestWindowsHandleRenameAndPrivateStaging(t *testing.T) {
	root := t.TempDir()
	stage, err := privateDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	guards, err := guardDirectories(stage)
	if err != nil {
		t.Fatal(err)
	}
	defer closeFiles(guards)
	if err = validatePrivateDirectory(guards[len(guards)-1]); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(stage, "locked.exe")
	if err = os.WriteFile(path, []byte("dummy bytes, not executable"), 0600); err != nil {
		t.Fatal(err)
	}
	locked, err := openLocked(path, true)
	if err != nil {
		t.Fatal(err)
	}
	defer locked.Close()
	if f, err := os.OpenFile(path, os.O_WRONLY, 0600); err == nil {
		f.Close()
		t.Fatal("deny-write handle allowed writer")
	}
	if err = os.Remove(path); err == nil {
		t.Fatal("deny-delete handle allowed deletion")
	}
	renamed := filepath.Join(stage, "renamed.exe")
	if err = renameHandle(locked, renamed, false); err != nil {
		t.Fatal("own DELETE handle cannot rename", err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("old name remains")
	}
	id1, _ := idOf(locked)
	if id1.Volume == 0 {
		t.Fatal("missing volume identity")
	}
}
func TestWindowsTrustRejectsUnsignedFixture(t *testing.T) {
	path := filepath.Join(t.TempDir(), "未簽章.exe")
	if err := os.WriteFile(path, imageFixture(0x8664, false), 0600); err != nil {
		t.Fatal(err)
	}
	identity, err := (WindowsIdentityVerifier{}).VerifyIdentity(path)
	if err == nil || identity.Trusted {
		t.Fatal("unsigned executable accepted", identity, err)
	}
}
func TestWindowsTrustedReleaseReadOnlyFixture(t *testing.T) {
	path := os.Getenv("RTA_UPDATE_TRUST_FIXTURE")
	if path == "" {
		t.Skip("positive signed fixture not supplied; no signing or real application execution is performed")
	}
	identity, err := (WindowsIdentityVerifier{}).VerifyIdentity(path)
	if err != nil || !identity.Trusted || identity.PublisherSubject == "" {
		t.Fatal("read-only signed fixture failed trust/version policy", err)
	}
	if _, err = ParseVersion(identity.Version); err != nil {
		t.Fatal(err)
	}
}
func TestWindowsPathRejections(t *testing.T) {
	for _, path := range []string{`relative.exe`, `\\server\share\x.exe`, `C:\file.exe:stream`, `C:\trailing.\app.exe`, `C:\a\..\b.exe`} {
		if localPath(path) == nil {
			t.Errorf("accepted %q", path)
		}
	}
	root := t.TempDir()
	actual := filepath.Join(root, "actual")
	_ = os.Mkdir(actual, 0700)
	link := filepath.Join(root, "junction")
	// Directory junctions do not require SeCreateSymbolicLinkPrivilege.
	if err := makeSandboxJunction(link, actual); err != nil {
		t.Fatal("sandbox junction creation", err)
	}
	guards, err := guardDirectories(link)
	closeFiles(guards)
	if err == nil {
		t.Fatal("reparse path accepted")
	}
}

func TestWindowsReadinessBoundAndAuthentication(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	r, w := io.Pipe()
	defer r.Close()
	defer w.Close()
	_, err := readFrame(ctx, bufio.NewReader(r))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
