// Package securestore provides replaceable OS-backed secret storage for the
// desktop application. Profile metadata deliberately lives outside this
// package because it is not secret.
package securestore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const appDirectoryName = "RTA Excel Filler"

var (
	// ErrNotFound means no credentials have been saved for a profile.
	ErrNotFound = errors.New("secure value not found")
	// ErrUnsupported means the native secure store is unavailable on this OS.
	ErrUnsupported = errors.New("native secure storage is unsupported on this platform")
)

// Credential contains the two values stored as a single Windows Credential
// Manager item. Neither value is written to profile metadata or logs.
type Credential struct {
	Account  string
	Password string
}

// CredentialStore abstracts Windows Credential Manager for tests and future
// platforms.
type CredentialStore interface {
	Get(profileID string) (Credential, error)
	Put(profileID string, credential Credential) error
	Delete(profileID string) error
}

// DataProtector abstracts Windows DPAPI. Implementations must return newly
// allocated byte slices that callers may overwrite independently.
type DataProtector interface {
	Protect(plaintext []byte) ([]byte, error)
	Unprotect(ciphertext []byte) ([]byte, error)
}

// Native groups the OS-backed capabilities used by the desktop backend.
type Native struct {
	Root        string
	Credentials CredentialStore
	Protector   DataProtector
}

// DefaultRoot returns the per-user application data directory.
func DefaultRoot() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate per-user application data: %w", err)
	}
	if strings.TrimSpace(base) == "" {
		return "", errors.New("locate per-user application data: empty directory")
	}
	return filepath.Join(base, appDirectoryName), nil
}

// NewNative creates the OS-backed vault and protector. Passing an empty root
// selects the current user's application data directory.
func NewNative(root string) (*Native, error) {
	if strings.TrimSpace(root) == "" {
		var err error
		root, err = DefaultRoot()
		if err != nil {
			return nil, err
		}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve application data directory: %w", err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create application data directory: %w", err)
	}
	credentials, err := newNativeCredentialStore()
	if err != nil {
		return nil, err
	}
	protector, err := newNativeProtector()
	if err != nil {
		return nil, err
	}
	return &Native{Root: root, Credentials: credentials, Protector: protector}, nil
}

// CookieStore returns an independently encrypted cookie store for a profile.
func (n *Native) CookieStore(profileID string) (*EncryptedFileStore, error) {
	if n == nil || n.Protector == nil {
		return nil, errors.New("secure storage is not initialized")
	}
	if err := validateProfileID(profileID); err != nil {
		return nil, err
	}
	return NewEncryptedFileStore(filepath.Join(n.Root, "cookies", profileID+".bin"), n.Protector)
}

// DeleteCookie removes only the selected profile's encrypted cookie file.
func (n *Native) DeleteCookie(profileID string) error {
	store, err := n.CookieStore(profileID)
	if err != nil {
		return err
	}
	return store.Delete()
}

func validateProfileID(profileID string) error {
	if profileID == "" || len(profileID) > 80 {
		return errors.New("invalid profile identifier")
	}
	for _, character := range profileID {
		if unicode.IsLetter(character) || unicode.IsDigit(character) || character == '-' || character == '_' {
			continue
		}
		return errors.New("invalid profile identifier")
	}
	return nil
}
