package portableupdate

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeOS struct {
	calls    []string
	fail     map[string]bool
	recorded Result
	restarts int
}

func (f *fakeOS) call(name string) error {
	f.calls = append(f.calls, name)
	if f.fail[name] {
		return errors.New(name)
	}
	return nil
}
func (f *fakeOS) Prepare(context.Context) error    { return f.call("prepare") }
func (f *fakeOS) Ready(context.Context) error      { return f.call("ready") }
func (f *fakeOS) WaitParent(context.Context) error { return f.call("wait") }
func (f *fakeOS) Verify(context.Context) error     { return f.call("verify") }
func (f *fakeOS) MoveOld() error                   { return f.call("backup") }
func (f *fakeOS) MoveNew() error                   { return f.call("replace") }
func (f *fakeOS) Restart() error {
	f.restarts++
	if f.restarts > 1 {
		return f.call("restart-old")
	}
	return f.call("restart")
}
func (f *fakeOS) RestoreOld() error     { return f.call("rollback") }
func (f *fakeOS) Record(r Result) error { f.recorded = r; return f.call("record") }
func TestTransactionFaults(t *testing.T) {
	all := []string{"prepare", "ready", "wait", "verify", "backup", "replace", "restart"}
	for i, phase := range all {
		t.Run(phase, func(t *testing.T) {
			os := &fakeOS{fail: map[string]bool{phase: true}}
			result, err := RunTransaction(context.Background(), os)
			if err == nil || os.recorded.Error == "" {
				t.Fatal("failure not retained")
			}
			expected := append([]string{}, all[:i+1]...)
			if i >= 5 {
				expected = append(expected, "rollback", "restart-old")
				if phase == "replace" {
					expected[len(expected)-1] = "restart"
				}
				if !result.RolledBack {
					t.Fatal("did not restore old executable")
				}
			}
			expected = append(expected, "record")
			if !reflect.DeepEqual(os.calls, expected) {
				t.Fatalf("calls %v want %v", os.calls, expected)
			}
		})
	}
	os := &fakeOS{}
	result, err := RunTransaction(context.Background(), os)
	if err != nil || result.Phase != "complete" || !reflect.DeepEqual(os.calls, append(all, "record")) {
		t.Fatal(result, err, os.calls)
	}
}
func TestTransactionRollbackFailureRetainsBackup(t *testing.T) {
	os := &fakeOS{fail: map[string]bool{"replace": true, "rollback": true}}
	result, err := RunTransaction(context.Background(), os)
	if err == nil || result.RolledBack || os.restarts != 0 {
		t.Fatal(result, err, os.calls)
	}
}
func TestTransactionRecordFailure(t *testing.T) {
	os := &fakeOS{fail: map[string]bool{"record": true}}
	if _, err := RunTransaction(context.Background(), os); err == nil {
		t.Fatal("lost diagnostic error")
	}
}
func TestTransactionCancellationDoesNotReplace(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	os := &fakeOS{}
	if _, err := RunTransaction(ctx, os); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(os.calls, []string{"record"}) {
		t.Fatal(os.calls)
	}
}

type fakeVerifier map[string]Identity

func (f fakeVerifier) VerifyIdentity(path string) (Identity, error) {
	v, ok := f[path]
	if !ok {
		return Identity{}, errors.New("missing")
	}
	return v, nil
}
func TestPublisherPolicy(t *testing.T) {
	current := "C:\\使用者\\銷售 app.exe"
	staged := "C:\\使用者\\更新\\candidate.exe"
	for _, tc := range []struct {
		name      string
		old, next Identity
		wantErr   bool
	}{
		{"same subject rotating certificate", Identity{true, "CN=Publisher", "0.4.5"}, Identity{true, "CN=Publisher", "0.5.0"}, false},
		{"unsigned current", Identity{false, "CN=Publisher", "0.4.5"}, Identity{true, "CN=Publisher", "0.5.0"}, true},
		{"unsigned candidate", Identity{true, "CN=Publisher", "0.4.5"}, Identity{false, "CN=Publisher", "0.5.0"}, true},
		{"different publisher", Identity{true, "CN=Publisher", "0.4.5"}, Identity{true, "CN=Other", "0.5.0"}, true},
		{"empty publisher", Identity{true, "", "0.4.5"}, Identity{true, "", "0.5.0"}, true},
		{"wrong version", Identity{true, "CN=Publisher", "0.4.5"}, Identity{true, "CN=Publisher", "0.5.1"}, true},
		{"development", Identity{true, "CN=Publisher", "dev"}, Identity{true, "CN=Publisher", "0.5.0"}, true},
		{"downgrade", Identity{true, "CN=Publisher", "0.6.0"}, Identity{true, "CN=Publisher", "0.5.0"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := VerifyPublisher(fakeVerifier{current: tc.old, staged: tc.next}, current, staged, "0.5.0")
			if (err != nil) != tc.wantErr {
				t.Fatal(err)
			}
		})
	}
	if VerifyPublisher(nil, current, staged, "0.5.0") == nil {
		t.Fatal("accepted absent verifier")
	}
}
