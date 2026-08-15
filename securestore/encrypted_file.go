package securestore

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

const maximumEncryptedCookieBytes = 32 << 20

// EncryptedFileStore implements rtasales.CookieStore with an injected OS data
// protector. Only protected bytes are ever written to disk.
type EncryptedFileStore struct {
	path      string
	protector DataProtector
	mu        sync.Mutex
}

func NewEncryptedFileStore(path string, protector DataProtector) (*EncryptedFileStore, error) {
	if protector == nil {
		return nil, errors.New("data protector is required")
	}
	if path == "" {
		return nil, errors.New("encrypted file path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve encrypted store path: %w", err)
	}
	return &EncryptedFileStore{path: absolute, protector: protector}, nil
}

func (s *EncryptedFileStore) Load() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open encrypted cookie store: %w", err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect encrypted cookie store: %w", err)
	}
	if info.Size() < 0 || info.Size() > maximumEncryptedCookieBytes {
		return nil, errors.New("encrypted cookie store exceeds 32 MiB")
	}
	ciphertext, err := io.ReadAll(io.LimitReader(file, maximumEncryptedCookieBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read encrypted cookie store: %w", err)
	}
	if len(ciphertext) > maximumEncryptedCookieBytes {
		return nil, errors.New("encrypted cookie store exceeds 32 MiB")
	}
	plaintext, err := s.protector.Unprotect(ciphertext)
	clearBytes(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt cookie store: %w", err)
	}
	return plaintext, nil
}

func (s *EncryptedFileStore) Save(plaintext []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ciphertext, err := s.protector.Protect(plaintext)
	if err != nil {
		return fmt.Errorf("encrypt cookie store: %w", err)
	}
	defer clearBytes(ciphertext)
	if len(ciphertext) > maximumEncryptedCookieBytes {
		return errors.New("encrypted cookie store exceeds 32 MiB")
	}
	parent := filepath.Dir(s.path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create encrypted cookie directory: %w", err)
	}
	temporary, err := os.CreateTemp(parent, ".cookie-*.tmp")
	if err != nil {
		return fmt.Errorf("create encrypted cookie update: %w", err)
	}
	temporaryName := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("secure encrypted cookie update: %w", err)
	}
	if _, err := io.Copy(temporary, bytes.NewReader(ciphertext)); err != nil {
		return fmt.Errorf("write encrypted cookie update: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync encrypted cookie update: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close encrypted cookie update: %w", err)
	}
	if err := replaceFile(temporaryName, s.path); err != nil {
		return fmt.Errorf("replace encrypted cookie store: %w", err)
	}
	committed = true
	return nil
}

func (s *EncryptedFileStore) Delete() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("delete encrypted cookie store: %w", err)
	}
	return nil
}

func clearBytes(values []byte) {
	for index := range values {
		values[index] = 0
	}
}
