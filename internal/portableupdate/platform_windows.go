//go:build windows

package portableupdate

import (
	"context"
	"crypto/subtle"
	"debug/pe"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

type helperConfig struct {
	Target        string
	Directory     string
	CWD           string
	ParentPID     uint32
	ParentCreated windows.Filetime
	TargetID      fileID
	CandidateID   fileID
	DirectoryID   fileID
	Hash          [32]byte
	CurrentHash   [32]byte
	Version       string
	Token         string
}

type windowsTransaction struct {
	config             helperConfig
	verifier           IdentityVerifier
	ready              func(context.Context) error
	launch             func(string, string) error
	self               string
	parent             windows.Handle
	guards             []*os.File
	old, next          *os.File
	directoryValidated bool
	installed          bool
	restored           bool
	restoredID         fileID
	currentIdentity    Identity
	// Only package tests can inject operation failures; no JSON/UI bypass exists.
	rename func(*os.File, string, bool) error
}

func newWindowsTransaction(c helperConfig) *windowsTransaction {
	return &windowsTransaction{config: c, verifier: WindowsIdentityVerifier{}, launch: restartExact, rename: renameHandle}
}
func (p *windowsTransaction) Close() {
	closeFiles([]*os.File{p.old, p.next})
	p.old = nil
	p.next = nil
	closeFiles(p.guards)
	p.guards = nil
	if p.parent != 0 {
		_ = windows.CloseHandle(p.parent)
		p.parent = 0
	}
}
func (p *windowsTransaction) candidatePath() string {
	return filepath.Join(p.config.Directory, "candidate.exe")
}
func (p *windowsTransaction) backupPath() string {
	return filepath.Join(p.config.Directory, "previous.exe")
}
func (p *windowsTransaction) Prepare(ctx context.Context) error {
	c := p.config
	if token, err := hex.DecodeString(c.Token); err != nil || len(token) != 32 {
		return errors.New("invalid helper authentication token")
	}
	if err := localPath(c.Target); err != nil {
		return err
	}
	if err := localPath(c.Directory); err != nil {
		return err
	}
	if err := localPath(c.CWD); err != nil {
		return err
	}
	if filepath.Dir(c.Directory) != filepath.Dir(c.Target) || !strings.HasPrefix(filepath.Base(c.Directory), ".rta-update-") {
		return errors.New("staging must be private and adjacent to target")
	}
	if !strings.EqualFold(filepath.Ext(c.Target), ".exe") {
		return errors.New("target is not executable")
	}
	guards, err := guardDirectories(c.Directory)
	if err != nil {
		return err
	}
	p.guards = guards
	dir := guards[len(guards)-1]
	id, err := idOf(dir)
	if err != nil || id != c.DirectoryID {
		return errors.New("staging directory identity changed")
	}
	if err = validatePrivateDirectory(dir); err != nil {
		return err
	}
	p.directoryValidated = true
	cwdGuards, err := guardDirectories(c.CWD)
	if err != nil {
		return err
	}
	p.guards = append(p.guards, cwdGuards...)
	if _, err = os.Lstat(p.backupPath()); !os.IsNotExist(err) {
		return errors.New("backup already exists or cannot be reserved")
	}
	if p.self == "" {
		p.self, err = os.Executable()
		if err != nil {
			return err
		}
	}
	if !strings.EqualFold(p.self, filepath.Join(c.Directory, "helper.exe")) {
		return errors.New("helper is not the private copied executable")
	}
	helper, err := openLocked(p.self, false)
	if err != nil {
		return err
	}
	helperHash, err := hashFile(helper)
	_ = helper.Close()
	if err != nil || helperHash != c.CurrentHash {
		return errors.New("copied helper hash differs from current executable")
	}
	p.parent, err = windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, c.ParentPID)
	if err != nil {
		return err
	}
	created, err := processCreated(p.parent)
	if err != nil || created != c.ParentCreated {
		return errors.New("parent process identity changed")
	}
	name := make([]uint16, 32768)
	size := uint32(len(name))
	if err = windows.QueryFullProcessImageName(p.parent, 0, &name[0], &size); err != nil {
		return err
	}
	if !strings.EqualFold(windows.UTF16ToString(name[:size]), c.Target) {
		return errors.New("parent is not the exact target executable")
	}
	state, err := windows.WaitForSingleObject(p.parent, 0)
	if err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		return errors.New("parent must be alive before readiness")
	}
	return p.verify(ctx, false)
}
func processCreated(h windows.Handle) (windows.Filetime, error) {
	var created, exit, kernel, user windows.Filetime
	err := windows.GetProcessTimes(h, &created, &exit, &kernel, &user)
	return created, err
}
func (p *windowsTransaction) Ready(ctx context.Context) error {
	if p.ready == nil {
		return errors.New("private readiness channel unavailable")
	}
	return p.ready(ctx)
}
func (p *windowsTransaction) WaitParent(ctx context.Context) error {
	if p.parent == 0 {
		return errors.New("missing parent handle")
	}
	// Bound even callers passing Background. No process termination is ever used.
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		state, err := windows.WaitForSingleObject(p.parent, 25)
		if err != nil {
			return err
		}
		if state == windows.WAIT_OBJECT_0 {
			return nil
		}
		if state != uint32(windows.WAIT_TIMEOUT) {
			return fmt.Errorf("unexpected parent wait status %d", state)
		}
	}
}
func (p *windowsTransaction) Verify(ctx context.Context) error { return p.verify(ctx, true) }
func (p *windowsTransaction) verify(ctx context.Context, retain bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	old, err := openLocked(p.config.Target, false)
	if err != nil {
		return err
	}
	defer old.Close()
	next, err := openLocked(p.candidatePath(), false)
	if err != nil {
		return err
	}
	defer next.Close()
	if err = validateTargetBoundary(old, p.config.Target, p.guards); err != nil {
		return err
	}
	if err = p.checkLocked(old, next); err != nil {
		return err
	}
	if err = VerifyPublisher(p.verifier, p.config.Target, p.candidatePath(), p.config.Version); err != nil {
		return err
	}
	p.currentIdentity, err = p.verifier.VerifyIdentity(p.config.Target)
	if err != nil || !p.currentIdentity.Trusted || p.currentIdentity.PublisherSubject == "" {
		return errors.New("current launch identity unavailable")
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if !retain {
		return nil
	}
	// Version APIs open by path without FILE_SHARE_DELETE. Close read-only trust
	// handles before acquiring DELETE handles. Recheck both ID AND bytes under
	// the new deny-write/delete handles; no trust decision spans mutable bytes.
	_ = next.Close()
	_ = old.Close()
	p.old, err = openLocked(p.config.Target, true)
	if err != nil {
		return err
	}
	p.next, err = openLocked(p.candidatePath(), true)
	if err != nil {
		return err
	}
	return p.checkLocked(p.old, p.next)
}
func (p *windowsTransaction) checkLocked(old, next *os.File) error {
	oldID, err := idOf(old)
	if err != nil || oldID != p.config.TargetID {
		return errors.New("target identity changed")
	}
	nextID, err := idOf(next)
	if err != nil || nextID != p.config.CandidateID {
		return errors.New("candidate identity changed")
	}
	if oldID.Volume != p.config.DirectoryID.Volume || nextID.Volume != oldID.Volume {
		return errors.New("update files are not on the same volume")
	}
	sum, err := hashFile(old)
	if err != nil || subtle.ConstantTimeCompare(sum[:], p.config.CurrentHash[:]) != 1 {
		return errors.New("current executable changed")
	}
	oldStat, err := old.Stat()
	if err != nil {
		return err
	}
	if err = VerifyImage(old, oldStat.Size(), p.config.CurrentHash, pe.IMAGE_FILE_MACHINE_AMD64); err != nil {
		return err
	}
	stat, err := next.Stat()
	if err != nil {
		return err
	}
	return VerifyImage(next, stat.Size(), p.config.Hash, pe.IMAGE_FILE_MACHINE_AMD64)
}
func (p *windowsTransaction) MoveOld() error {
	if p.old == nil || p.next == nil {
		return errors.New("no verified rename handles")
	}
	return p.rename(p.old, p.backupPath(), false)
}
func (p *windowsTransaction) MoveNew() error {
	if p.next == nil {
		return errors.New("no candidate handle")
	}
	err := p.rename(p.next, p.config.Target, false)
	if err == nil {
		p.installed = true
	}
	return err
}
func (p *windowsTransaction) Restart() error {
	if p.next == nil {
		return errors.New("verified launch source handle unavailable")
	}
	// Revalidate effective owner/DACL policy while the verified DELETE handle
	// still excludes content writers/deleters. A read/deny-delete handle cannot
	// coexist with it; the enforced cross-principal ACL boundary covers this
	// necessary close/reopen interval, for BOTH update and recovery launches.
	if err := validateTargetBoundary(p.next, p.config.Target, p.guards); err != nil {
		return err
	}
	_ = p.next.Close()
	p.next = nil
	locked, err := openLocked(p.config.Target, false)
	if err != nil {
		return err
	}
	defer locked.Close()
	if err = validateTargetBoundary(locked, p.config.Target, p.guards); err != nil {
		return err
	}
	expectedID, expectedHash, expectedVersion := p.config.CandidateID, p.config.Hash, p.config.Version
	if p.restored {
		expectedID, expectedHash, expectedVersion = p.restoredID, p.config.CurrentHash, p.currentIdentity.Version
	}
	id, err := idOf(locked)
	if err != nil || id != expectedID {
		return errors.New("launch target identity changed")
	}
	stat, err := locked.Stat()
	if err != nil {
		return err
	}
	if err = VerifyImage(locked, stat.Size(), expectedHash, pe.IMAGE_FILE_MACHINE_AMD64); err != nil {
		return err
	}
	identity, err := p.verifier.VerifyIdentity(p.config.Target)
	if err != nil {
		return err
	}
	if !identity.Trusted || identity.PublisherSubject != p.currentIdentity.PublisherSubject || identity.Version != expectedVersion {
		return errors.New("launch target signed identity changed")
	}
	// Actual Windows sandbox tests demonstrate CreateProcess compatibility with
	// this read-only FILE_SHARE_READ handle. It remains held through launch.
	return p.launch(p.config.Target, p.config.CWD)
}
func restartExact(target, cwd string) error {
	if err := localPath(target); err != nil {
		return err
	}
	if err := localPath(cwd); err != nil {
		return err
	}
	// Absolute application name, no shell, no forwarded helper/UI arguments.
	// StartProcess errors mean CreateProcess failed (no child); successful launch
	// is the commit point. A later child crash is not interpreted as rollback.
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer null.Close()
	process, err := os.StartProcess(target, []string{target}, &os.ProcAttr{Dir: cwd, Env: os.Environ(), Files: []*os.File{null, null, null}})
	if err != nil {
		return err
	}
	_ = process.Release()
	return nil
}
func (p *windowsTransaction) RestoreOld() error {
	if p.old == nil {
		return errors.New("backup handle unavailable")
	}
	if err := validateTargetBoundary(nil, p.config.Target, p.guards); err != nil {
		return err
	}
	if p.next != nil {
		_ = p.next.Close()
		p.next = nil
	}
	// Do not overwrite an unrelated file recreated at the target during failure.
	if _, err := os.Lstat(p.config.Target); err == nil {
		if !p.installed {
			return errors.New("target unexpectedly recreated; preserve backup for manual recovery")
		}
		atTarget, err := openLocked(p.config.Target, false)
		if err != nil {
			return err
		}
		id, idErr := idOf(atTarget)
		_ = atTarget.Close()
		if idErr != nil || id != p.config.CandidateID {
			return errors.New("target identity changed; preserve backup for manual recovery")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	recovery := filepath.Join(p.config.Directory, "recovery.exe")
	// Copy from the still-locked old handle. The unique good backup is NEVER
	// consumed by rollback, including when restoring/restarting subsequently fails.
	if err := copyExclusive(p.old, recovery); err != nil {
		return err
	}
	restored, err := openLocked(recovery, true)
	if err != nil {
		return err
	}
	sum, err := hashFile(restored)
	if err != nil || sum != p.config.CurrentHash {
		restored.Close()
		return errors.New("recovery copy verification failed")
	}
	id, err := idOf(restored)
	if err != nil {
		restored.Close()
		return err
	}
	if err = p.rename(restored, p.config.Target, true); err != nil {
		restored.Close()
		return err
	}
	p.restored = true
	p.restoredID = id
	p.next = restored
	return nil
}
func (p *windowsTransaction) Record(result Result) error {
	if !p.directoryValidated {
		return nil
	} // Never write to an unvalidated input path.
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(p.config.Directory, "result.json")
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = out.Write(data); err == nil {
		err = out.Sync()
	}
	return errors.Join(err, out.Close())
}
