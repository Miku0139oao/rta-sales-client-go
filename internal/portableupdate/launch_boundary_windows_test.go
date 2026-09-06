//go:build windows

package portableupdate

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func setSandboxACL(t *testing.T, path, extra string) {
	t.Helper()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;" + user.User.Sid.String() + ")" + extra)
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
}
func TestWindowsLaunchACLPrincipalModel(t *testing.T) {
	current := "S-1-5-21-1-2-3-1001"
	other := "S-1-5-21-1-2-3-1002"
	for _, sid := range []string{other, "S-1-1-0", "S-1-5-32-545", "S-1-5-11"} {
		if trustedLaunchPrincipal(sid, current) {
			t.Fatalf("unprivileged owner %s trusted", sid)
		}
		for _, mask := range []uint32{0x2, 0x4, 0x10, 0x40, 0x100, 0x10000, 0x40000, 0x80000, 0x40000000, 0x10000000} {
			if !unsafeLaunchGrant(sid, current, mask, true, true) {
				t.Fatalf("target parent grant accepted: %s %#x", sid, mask)
			}
			if !unsafeLaunchGrant(sid, current, mask, false, false) {
				t.Fatalf("file grant accepted: %s %#x", sid, mask)
			}
		}
		if unsafeLaunchGrant(sid, current, 0x1200a9, true, true) {
			t.Fatal("read/execute grant rejected")
		}
	}
	if !unsafeLaunchGrant(other, current, 0x40000, true, false) || !unsafeLaunchGrant(other, current, 0x100, true, false) {
		t.Fatal("ancestor DACL/metadata writer accepted")
	}
	for _, sid := range []string{current, "S-1-5-18", "S-1-5-32-544"} {
		if !trustedLaunchPrincipal(sid, current) {
			t.Fatal("trusted owner rejected")
		}
	}
}
func TestWindowsUnsafeTargetACLRejected(t *testing.T) {
	dummy := buildSandboxDummy(t)
	for _, tc := range []struct {
		name, ace string
		file      bool
	}{
		{"parent delete-child", "(A;;0x40;;;WD)", false},
		{"parent create-file", "(A;;0x2;;;WD)", false},
		{"parent write-DACL", "(A;;0x40000;;;WD)", false},
		{"parent write-owner", "(A;;0x80000;;;WD)", false},
		{"file write-data", "(A;;0x2;;;WD)", true},
		{"file write-DACL", "(A;;0x40000;;;WD)", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, _ := sandbox(t, dummy)
			path := filepath.Dir(p.config.Target)
			if tc.file {
				path = p.config.Target
			}
			setSandboxACL(t, path, tc.ace)
			if err := p.Prepare(context.Background()); err == nil {
				t.Fatal("unsafe ACL admitted before readiness")
			}
			if _, err := os.Stat(p.backupPath()); !os.IsNotExist(err) {
				t.Fatal("unsafe target was moved")
			}
		})
	}
}
func TestWindowsLaunchReadLockAndRollback(t *testing.T) {
	dummy := buildSandboxDummy(t)
	for _, rollback := range []bool{false, true} {
		name := "updated"
		if rollback {
			name = "rollback"
		}
		t.Run(name, func(t *testing.T) {
			p, _ := sandbox(t, dummy)
			calls := 0
			p.launch = func(target, cwd string) error {
				calls++
				if writer, err := os.OpenFile(target, os.O_WRONLY, 0600); err == nil {
					writer.Close()
					t.Error("launch allowed concurrent content writer")
				}
				if err := os.Remove(target); err == nil {
					t.Error("launch allowed deletion")
				}
				replacement := filepath.Join(cwd, "unrelated-replacement.exe")
				if err := os.WriteFile(replacement, []byte("not executable"), 0600); err != nil {
					return err
				}
				if err := os.Rename(replacement, target); err == nil {
					t.Error("launch allowed identity replacement")
				}
				if rollback && calls == 1 {
					return errors.New("injected first CreateProcess failure")
				}
				return restartExact(target, cwd) // REAL dummy CreateProcess under deny-delete read lock
			}
			result, err := RunTransaction(context.Background(), p)
			p.Close()
			if rollback {
				if err == nil || !result.RolledBack || calls != 2 {
					t.Fatal(result, err, calls)
				}
			} else if err != nil || calls != 1 {
				t.Fatal(result, err, calls)
			}
			waitMarker(t, p.config.CWD)
			backup, _ := os.ReadFile(p.backupPath())
			if !bytes.Equal(backup, dummy) {
				t.Fatal("good backup not retained")
			}
		})
	}
}
func TestWindowsReadDenyDeleteCannotOverlapRenameHandle(t *testing.T) {
	path := filepath.Join(safeSandboxRoot(t), "sharing.exe")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	rename, err := openLocked(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if reader, err := openLocked(path, false); err == nil {
		reader.Close()
		t.Fatal("unexpected Windows sharing semantics: test the transition before changing policy")
	}
	rename.Close()
	reader, err := openLocked(path, false)
	if err != nil {
		t.Fatal(err)
	}
	reader.Close()
}
func prepareMovedSandbox(t *testing.T, dummy []byte) *windowsTransaction {
	t.Helper()
	p, release := sandbox(t, dummy)
	if err := p.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	release()
	if err := p.WaitParent(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := p.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := p.MoveOld(); err != nil {
		t.Fatal(err)
	}
	if err := p.MoveNew(); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestWindowsLaunchRechecksACLIdentityAndSignature(t *testing.T) {
	dummy := buildSandboxDummy(t)
	for _, kind := range []string{"parent ACL", "file identity", "file hash", "signed publisher", "signed version"} {
		t.Run(kind, func(t *testing.T) {
			p := prepareMovedSandbox(t, dummy)
			launched := false
			p.launch = func(string, string) error { launched = true; return nil }
			switch kind {
			case "parent ACL":
				setSandboxACL(t, filepath.Dir(p.config.Target), "(A;;0x40;;;WD)")
			case "file identity":
				// Model an already-substituted same-user file, not a production bypass.
				p.next.Close()
				if err := os.Remove(p.config.Target); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p.config.Target, append(append([]byte{}, dummy...), []byte("candidate-marker")...), 0600); err != nil {
					t.Fatal(err)
				}
				var err error
				p.next, err = openLocked(p.config.Target, true)
				if err != nil {
					t.Fatal(err)
				}
			case "file hash":
				p.next.Close()
				writer, err := os.OpenFile(p.config.Target, os.O_WRONLY|os.O_APPEND, 0600)
				if err != nil {
					t.Fatal(err)
				}
				_, err = writer.Write([]byte("changed bytes, same file ID"))
				writer.Close()
				if err != nil {
					t.Fatal(err)
				}
				p.next, err = openLocked(p.config.Target, true)
				if err != nil {
					t.Fatal(err)
				}
			case "signed publisher":
				p.verifier = fakeVerifier{p.config.Target: {Trusted: true, PublisherSubject: "CN=Different", Version: p.config.Version}}
			case "signed version":
				p.verifier = fakeVerifier{p.config.Target: {Trusted: true, PublisherSubject: p.currentIdentity.PublisherSubject, Version: "99.0.0"}}
			}
			if err := p.Restart(); err == nil || launched {
				t.Fatal("unverified/unsafe path launched", err)
			}
		})
	}
}
