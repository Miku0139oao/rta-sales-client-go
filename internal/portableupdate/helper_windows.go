//go:build windows

package portableupdate

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

const HelperFlag = "--portable-update-helper"

// WindowsStaging is created from the current process, never frontend paths.
// All staging is retained for diagnostics/manual recovery; Close releases locks
// but does not remove downloaded files, the helper or the sole good backup.
type WindowsStaging struct {
	config   helperConfig
	guards   []*os.File
	complete bool
	started  bool
}

func NewWindowsStaging() (*WindowsStaging, error) {
	target, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return newStaging(target, cwd, WindowsIdentityVerifier{})
}
func newStaging(target, cwd string, verifier IdentityVerifier) (*WindowsStaging, error) {
	guards, err := guardDirectories(filepath.Dir(target))
	if err != nil {
		return nil, err
	}
	old, err := openLocked(target, false)
	if err != nil {
		closeFiles(guards)
		return nil, err
	}
	defer old.Close()
	if err = validateTargetBoundary(old, target, guards); err != nil {
		closeFiles(guards)
		return nil, err
	}
	identity, err := verifier.VerifyIdentity(target)
	if err != nil || !identity.Trusted || identity.PublisherSubject == "" {
		closeFiles(guards)
		return nil, errors.New("cannot stage from an untrusted current executable")
	}
	if _, err = ParseVersion(identity.Version); err != nil {
		closeFiles(guards)
		return nil, err
	}
	directory, err := privateDirectory(filepath.Dir(target))
	if err != nil {
		closeFiles(guards)
		return nil, err
	}
	stageGuards, err := guardDirectories(directory)
	if err != nil {
		closeFiles(guards)
		return nil, err
	}
	guards = append(guards, stageGuards...)
	s := &WindowsStaging{guards: guards, config: helperConfig{Target: target, Directory: directory, CWD: cwd, ParentPID: uint32(os.Getpid())}}
	fail := func(err error) (*WindowsStaging, error) { s.Close(); return nil, err }
	if err = validatePrivateDirectory(stageGuards[len(stageGuards)-1]); err != nil {
		return fail(err)
	}
	s.config.DirectoryID, err = idOf(stageGuards[len(stageGuards)-1])
	if err != nil {
		return fail(err)
	}
	s.config.TargetID, err = idOf(old)
	if err != nil {
		return fail(err)
	}
	s.config.CurrentHash, err = hashFile(old)
	if err != nil {
		return fail(err)
	}
	s.config.Token, err = nonce()
	if err != nil {
		return fail(err)
	}
	if err = copyExclusive(old, filepath.Join(directory, "helper.exe")); err != nil {
		return fail(err)
	}
	return s, nil
}
func (s *WindowsStaging) Close()            { closeFiles(s.guards); s.guards = nil }
func (s *WindowsStaging) Directory() string { return s.config.Directory }

// Download is explicit and takes only an immutable candidate returned by Check.
// Desktop installation calls this only after explicit consent and reservation.
func (s *WindowsStaging) Download(ctx context.Context, client *Client, candidate Candidate) error {
	if client == nil || s.complete || s.started || len(s.guards) == 0 {
		return errors.New("staging is closed, already used or missing client")
	}
	path := filepath.Join(s.config.Directory, "candidate.exe")
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	sum, err := client.Download(ctx, candidate, out)
	if err == nil {
		err = out.Sync()
	}
	err = errors.Join(err, out.Close())
	if err != nil {
		return err
	}
	locked, err := openLocked(path, false)
	if err != nil {
		return err
	}
	defer locked.Close()
	s.config.CandidateID, err = idOf(locked)
	if err != nil {
		return err
	}
	s.config.Hash = sum
	s.config.Version = candidate.Version()
	// Locked candidate path cannot change during the path-based trust check.
	if err = VerifyPublisher(WindowsIdentityVerifier{}, s.config.Target, path, candidate.Version()); err != nil {
		return err
	}
	s.complete = true
	return nil
}

// HelperSession is a readiness receipt. The lifecycle owner must reserve the
// backend gate, call Commit, then quit normally WITHOUT holding any mutex.
// Cancel never terminates a process. The helper has its own bounded timeout.
type HelperSession struct {
	mu            sync.Mutex
	input         io.WriteCloser
	token         string
	committed     bool
	commitStarted bool
	output        io.ReadCloser
	reader        *bufio.Reader
	commitTimeout time.Duration // test-injected; production uses five seconds
	done          <-chan struct{}
}

func (s *HelperSession) Commit() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.input == nil || s.reader == nil || s.committed || s.commitStarted {
		return errors.New("helper receipt is closed or commit was already attempted")
	}
	select {
	case <-s.done:
		return errors.New("helper exited before commit")
	default:
	}
	s.commitStarted = true
	budget := s.commitTimeout
	if budget <= 0 {
		budget = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	fail := func(err error) error { s.abortLocked(); return err }
	if _, err := io.WriteString(s.input, "commit:"+s.token+"\n"); err != nil {
		return fail(err)
	}
	line, err := readFrame(ctx, s.reader)
	if err != nil {
		return fail(fmt.Errorf("helper commit acknowledgement: %w", err))
	}
	if err = ctx.Err(); err != nil {
		return fail(err)
	}
	if line != "commit-accepted:"+s.token {
		return fail(errors.New("helper commit acknowledgement authentication failed"))
	}
	select {
	case <-s.done:
		return fail(errors.New("helper exited while acknowledging commit"))
	default:
	}
	s.committed = true
	if s.output != nil {
		_ = s.output.Close()
		s.output = nil
	}
	return nil
}

// Caller holds mu. Short control frames cannot fill the consumed config pipe.
// Closing our response handle also releases a timed-out readFrame goroutine.
func (s *HelperSession) abortLocked() {
	if s.input != nil {
		_, _ = io.WriteString(s.input, "cancel:"+s.token+"\n")
		_ = s.input.Close()
		s.input = nil
	}
	if s.output != nil {
		_ = s.output.Close()
		s.output = nil
	}
}
func (s *HelperSession) Cancel() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.abortLocked()
	if s.done == nil {
		return errors.New("helper exit receipt unavailable")
	}
	select {
	case <-s.done:
		return nil // broken pipe is harmless once exit is confirmed
	case <-time.After(2 * time.Second):
		return errors.New("helper cancellation sent; natural exit still pending")
	}
}
func (s *WindowsStaging) StartHelper(ctx context.Context) (*HelperSession, error) {
	if !s.complete || s.started || len(s.guards) == 0 {
		return nil, errors.New("no verified staging ready for helper")
	}
	created, err := processCreatedCurrent()
	if err != nil {
		return nil, err
	}
	s.config.ParentCreated = created
	s.started = true
	return startHelper(ctx, s.config)
}
func processCreatedCurrent() (windows.Filetime, error) {
	return processCreated(windows.CurrentProcess())
}
func startHelper(ctx context.Context, c helperConfig) (*HelperSession, error) {
	return startHelperChecked(ctx, c, WindowsIdentityVerifier{})
}
func startHelperChecked(ctx context.Context, c helperConfig, verifier IdentityVerifier) (*HelperSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	helper, err := openLocked(filepath.Join(c.Directory, "helper.exe"), false)
	if err != nil {
		return nil, err
	}
	defer helper.Close()
	sum, err := hashFile(helper)
	if err != nil || sum != c.CurrentHash {
		return nil, errors.New("helper changed before launch")
	}
	current, err := openLocked(c.Target, false)
	if err != nil {
		return nil, err
	}
	defer current.Close()
	guards, err := guardDirectories(filepath.Dir(c.Target))
	if err != nil {
		return nil, err
	}
	defer closeFiles(guards)
	if err = validateTargetBoundary(current, c.Target, guards); err != nil {
		return nil, err
	}
	sum, err = hashFile(current)
	if err != nil || sum != c.CurrentHash {
		return nil, errors.New("current executable changed before helper launch")
	}
	identity, err := verifier.VerifyIdentity(c.Target)
	if err != nil || !identity.Trusted || identity.PublisherSubject == "" {
		return nil, errors.New("current executable no longer trusted")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cmd := exec.Command(filepath.Join(c.Directory, "helper.exe"), HelperFlag)
	cmd.Dir = c.Directory
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		input.Close()
		output.Close()
		return nil, err
	}
	// Reap only our dedicated copied helper, never the parent or restarted app.
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	fail := func(err error) (*HelperSession, error) {
		input.Close()
		output.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
		return nil, err
	}
	data, err := json.Marshal(c)
	if err != nil {
		return fail(err)
	}
	if len(data) > 12000 {
		return fail(errors.New("helper configuration too large"))
	}
	if _, err = input.Write(append(data, '\n')); err != nil {
		return fail(err)
	}
	reader := bufio.NewReader(io.LimitReader(output, 256))
	line, err := readFrame(ctx, reader)
	if err != nil {
		return fail(err)
	}
	if line != "ready:"+c.Token {
		return fail(errors.New("helper readiness authentication failed"))
	}
	return &HelperSession{input: input, output: output, reader: reader, token: c.Token, done: done}, nil
}
func readFrame(ctx context.Context, reader *bufio.Reader) (string, error) {
	type answer struct {
		line string
		err  error
	}
	done := make(chan answer, 1)
	go func() { line, err := reader.ReadString('\n'); done <- answer{strings.TrimSuffix(line, "\n"), err} }()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case r := <-done:
		return r.line, r.err
	}
}

// DispatchHelper must be the first action in main. It never constructs the
// desktop app or acquires Wails' single-instance lock. Inputs arrive exclusively
// through inherited anonymous pipes, not command-line paths or frontend RPC.
func DispatchHelper(args []string) (bool, int) {
	if len(args) == 0 || args[0] != HelperFlag {
		return false, 0
	}
	if len(args) != 1 {
		return true, 2
	}
	for _, file := range []*os.File{os.Stdin, os.Stdout} {
		typeOfHandle, err := windows.GetFileType(windows.Handle(file.Fd()))
		if err != nil || typeOfHandle != windows.FILE_TYPE_PIPE {
			return true, 2
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	return true, dispatchHelperIO(ctx, cancel, os.Stdin, os.Stdout, newWindowsTransaction, 30*time.Second)
}

// Shared with sandbox protocol tests. The public dispatch ALWAYS uses the real
// Windows verifier/factory; neither arguments nor config can replace it.
func dispatchHelperIO(ctx context.Context, cancel context.CancelFunc, input io.Reader, output io.Writer, factory func(helperConfig) *windowsTransaction, commitWait time.Duration) int {
	reader := bufio.NewReader(io.LimitReader(input, 16384))
	line, err := readFrame(ctx, reader)
	if err != nil {
		return 2
	}
	var c helperConfig
	decoder := json.NewDecoder(strings.NewReader(line))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&c); err != nil {
		return 2
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		return 2
	}
	p := factory(c)
	defer p.Close()
	p.ready = func(ctx context.Context) error {
		readyCtx, stop := context.WithTimeout(ctx, commitWait)
		defer stop()
		if err := readyCtx.Err(); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(output, "ready:"+c.Token); err != nil {
			return err
		}
		line, err := readFrame(readyCtx, reader)
		if err != nil {
			return err
		}
		// readFrame's select may race a simultaneous frame and timer. An expired
		// readiness receipt never gets an ACK even if the pipe remains writable.
		if err = readyCtx.Err(); err != nil {
			return err
		}
		if line != "commit:"+c.Token {
			return errors.New("parent did not authorize helper commit")
		}
		if _, err = fmt.Fprintln(output, "commit-accepted:"+c.Token); err != nil {
			return err
		}
		go func() {
			line, err := reader.ReadString('\n')
			if err == nil && line == "cancel:"+c.Token+"\n" {
				cancel()
			}
			// EOF means the parent closed normally. The real process handle still
			// decides whether it is safe to replace, not pipe EOF or a PID poll.
		}()
		return nil
	}
	_, err = RunTransaction(ctx, p)
	if err != nil {
		return 1
	}
	return 0
}
