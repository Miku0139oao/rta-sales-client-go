//go:build !windows

package securestore

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFileVaultRoundTripDoesNotWritePlaintext(t *testing.T) {
	root := t.TempDir()
	protector, err := newFileProtector(filepath.Join(root, "vault.key"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := newFileCredentialStore(filepath.Join(root, "credentials.bin"), protector)
	if err != nil {
		t.Fatal(err)
	}
	want := Credential{Account: "shop-user", Password: "s3cret-pass"}
	if err := store.Put(testProfileID, want); err != nil {
		t.Fatal(err)
	}
	onDisk, err := os.ReadFile(filepath.Join(root, "credentials.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(onDisk, []byte("s3cret-pass")) || bytes.Contains(onDisk, []byte("shop-user")) {
		t.Fatal("file vault exposed plaintext credentials")
	}
	got, err := store.Get(testProfileID)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("credential mismatch: %#v", got)
	}
	if err := store.Delete(testProfileID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(testProfileID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
