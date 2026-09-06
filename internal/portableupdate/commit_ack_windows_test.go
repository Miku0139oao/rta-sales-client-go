//go:build windows

package portableupdate

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

type protocolOutput struct {
	out          io.Writer
	mode         string
	releaseReady <-chan struct{}
}

func (w protocolOutput) Write(data []byte) (int, error) {
	text := string(data)
	if strings.HasPrefix(text, "commit-accepted:") {
		if w.mode == "missing" {
			return len(data), nil
		}
		if w.mode == "bad" {
			_, err := io.WriteString(w.out, "commit-accepted:wrong-token\n")
			return len(data), err
		}
	}
	n, err := w.out.Write(data)
	if strings.HasPrefix(text, "ready:") && w.releaseReady != nil {
		<-w.releaseReady
	}
	return n, err
}

type protocolHarness struct {
	session     *HelperSession
	returned    chan int
	done        chan struct{}
	releaseExit chan struct{}
}

// Executes the actual DispatchHelper implementation below its stdin/argument
// validation. JSON parsing, Windows Prepare, readiness/ACK, parent wait and
// diagnostics are production code. Only identity verification is injected for
// the unsigned dummy. No CLI/env switch can inject it into public dispatch.
func newProtocolHarness(t *testing.T, dummy []byte, outputMode string, readyGate <-chan struct{}, commitWait time.Duration, retainPipes bool) *protocolHarness {
	t.Helper()
	p, _ := sandbox(t, dummy)
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	h := &protocolHarness{returned: make(chan int, 1), done: make(chan struct{}), releaseExit: make(chan struct{})}
	if !retainPipes {
		close(h.releaseExit)
	}
	t.Cleanup(func() {
		cancel()
		inW.Close()
		outR.Close()
		// Retained endpoints model a helper doing slow diagnostics after expiry.
		select {
		case <-h.releaseExit:
		default:
			close(h.releaseExit)
		}
		select {
		case <-h.done:
		case <-time.After(6 * time.Second):
			t.Error("sandbox protocol did not exit")
		}
	})
	go func() {
		code := dispatchHelperIO(ctx, cancel, inR, protocolOutput{out: outW, mode: outputMode, releaseReady: readyGate}, func(c helperConfig) *windowsTransaction { p.config = c; return p }, commitWait)
		h.returned <- code
		<-h.releaseExit
		inR.Close()
		outW.Close()
		close(h.done)
	}()
	config, err := json.Marshal(p.config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = inW.Write(append(config, '\n')); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(io.LimitReader(outR, 256))
	line, err := readFrame(ctx, reader)
	if err != nil || line != "ready:"+p.config.Token {
		t.Fatal("real dispatch never became ready", line, err)
	}
	h.session = &HelperSession{input: inW, output: outR, reader: reader, token: p.config.Token, done: h.done, commitTimeout: 500 * time.Millisecond}
	return h
}
func TestWindowsActualDispatchCommitAcknowledgement(t *testing.T) {
	dummy := buildSandboxDummy(t)
	t.Run("accepted ACK then cancellation while parent remains alive", func(t *testing.T) {
		h := newProtocolHarness(t, dummy, "", nil, time.Second, false)
		if err := h.session.Commit(); err != nil {
			t.Fatal(err)
		}
		if !h.session.committed {
			t.Fatal("ACK did not establish committed receipt")
		}
		if err := h.session.Commit(); err == nil {
			t.Fatal("duplicate commit accepted")
		}
		if err := h.session.Cancel(); err != nil {
			t.Fatal(err)
		}
		if code := <-h.returned; code != 1 {
			t.Fatal("cancelled helper should not replace live parent", code)
		}
	})
	for _, mode := range []string{"bad", "missing"} {
		t.Run(mode+" ACK never authorizes quit", func(t *testing.T) {
			h := newProtocolHarness(t, dummy, mode, nil, time.Second, false)
			wouldQuit := false
			if err := h.session.Commit(); err == nil {
				wouldQuit = true
			}
			if wouldQuit || h.session.committed {
				t.Fatal("write-only or unauthenticated success")
			}
			if err := h.session.Cancel(); err != nil {
				t.Fatal(err)
			}
			if code := <-h.returned; code != 1 {
				t.Fatal(code)
			}
		})
	}
	t.Run("expired readiness while input pipe remains writable", func(t *testing.T) {
		h := newProtocolHarness(t, dummy, "", nil, 20*time.Millisecond, true)
		if code := <-h.returned; code != 1 {
			t.Fatal("expiry did not fail actual dispatch", code)
		}
		// The helper has rejected readiness but has not closed its inherited pipe:
		// the formerly unsafe WriteString still succeeds. No ACK must mean failure.
		if err := h.session.Commit(); err == nil || h.session.committed {
			t.Fatal("late commit accepted", err)
		}
		close(h.releaseExit)
		if err := h.session.Cancel(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("frame queued across readiness deadline never gets ACK", func(t *testing.T) {
		gate := make(chan struct{})
		h := newProtocolHarness(t, dummy, "", gate, 20*time.Millisecond, false)
		time.Sleep(40 * time.Millisecond)
		queued := make(chan struct{}, 1)
		h.session.input = observedControl{WriteCloser: h.session.input, wrote: queued}
		result := make(chan error, 1)
		go func() { result <- h.session.Commit() }()
		<-queued
		close(gate) // actual helper now observes an expired deadline + queued input
		if err := <-result; err == nil || h.session.committed {
			t.Fatal("deadline/frame race accepted", err)
		}
		if err := h.session.Cancel(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("helper exits before Commit", func(t *testing.T) {
		h := newProtocolHarness(t, dummy, "", nil, 20*time.Millisecond, false)
		<-h.done
		if err := h.session.Commit(); err == nil {
			t.Fatal("exited helper accepted commit")
		}
		if err := h.session.Cancel(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("concurrent session cancellation does not deadlock missing ACK", func(t *testing.T) {
		h := newProtocolHarness(t, dummy, "missing", nil, time.Second, false)
		commit := make(chan error, 1)
		go func() { commit <- h.session.Commit() }()
		cancel := make(chan error, 1)
		go func() { cancel <- h.session.Cancel() }()
		select {
		case err := <-commit:
			if err == nil {
				t.Fatal("missing ACK accepted")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("Commit deadlocked")
		}
		select {
		case err := <-cancel:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("Cancel deadlocked")
		}
	})
}
func TestWindowsCommitFailureIsOneShot(t *testing.T) {
	done := make(chan struct{})
	close(done)
	s := &HelperSession{input: fakeControlWriter{}, reader: bufio.NewReader(strings.NewReader("")), done: done}
	if err := s.Commit(); err == nil {
		t.Fatal("helper already exited")
	}
	if err := s.Cancel(); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsCommitRejectsKnownExitAfterACK(t *testing.T) {
	done := make(chan struct{})
	s := &HelperSession{input: discardedControl{io.Discard}, reader: bufio.NewReader(exitReader{strings.NewReader("commit-accepted:token\n"), done}), token: "token", done: done}
	if err := s.Commit(); err == nil || s.committed {
		t.Fatal("known exited helper reported committed", err)
	}
	if err := s.Cancel(); err != nil {
		t.Fatal(err)
	}
}

type discardedControl struct{ io.Writer }

func (discardedControl) Close() error { return nil }

type exitReader struct {
	io.Reader
	done chan struct{}
}

func (r exitReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	select {
	case <-r.done:
	default:
		close(r.done)
	}
	return n, err
}

type observedControl struct {
	io.WriteCloser
	wrote chan struct{}
}

func (w observedControl) Write(p []byte) (int, error) {
	n, err := w.WriteCloser.Write(p)
	if err == nil {
		select {
		case w.wrote <- struct{}{}:
		default:
		}
	}
	return n, err
}

type fakeControlWriter struct{}

func (fakeControlWriter) Write(p []byte) (int, error) {
	return 0, errors.New("closed test control pipe")
}
func (fakeControlWriter) Close() error { return nil }
