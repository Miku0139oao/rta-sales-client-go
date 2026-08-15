package securestore

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const testProfileID = "123e4567-e89b-42d3-a456-426614174000"

func TestMemoryCredentialStoreLifecycle(t *testing.T) {
	vault := NewMemoryCredentialStore()
	want := Credential{Account: "test-account", Password: "test-password"}
	if err := vault.Put(testProfileID, want); err != nil {
		t.Fatal(err)
	}
	got, err := vault.Get(testProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("credential mismatch: %#v", got)
	}
	if err := vault.Delete(testProfileID); err != nil {
		t.Fatal(err)
	}
	if _, err := vault.Get(testProfileID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestEncryptedFileStoreDoesNotWritePlaintext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookies", "profile.bin")
	store, err := NewEncryptedFileStore(path, MemoryProtector{})
	if err != nil {
		t.Fatal(err)
	}
	want := []byte(`[{"Name":"session","Value":"plain-secret"}]`)
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(onDisk, []byte("plain-secret")) {
		t.Fatal("encrypted store exposed plaintext")
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("round trip mismatch: %q", got)
	}
	if err := store.Delete(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected encrypted store deletion, got %v", err)
	}
}

func TestNativeCookieStoreRejectsUnsafeProfileID(t *testing.T) {
	native := &Native{Root: t.TempDir(), Protector: MemoryProtector{}, Credentials: NewMemoryCredentialStore()}
	if _, err := native.CookieStore("../escape"); err == nil {
		t.Fatal("expected unsafe profile identifier rejection")
	}
}
