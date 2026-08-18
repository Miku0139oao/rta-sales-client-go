//go:build !windows

package securestore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	fileVaultKeyBytes   = 32
	fileVaultNonceBytes = 12
)

func newNativeCredentialStore(root string) (CredentialStore, error) {
	protector, err := newNativeProtector(root)
	if err != nil {
		return nil, err
	}
	return newFileCredentialStore(filepath.Join(root, "credentials.bin"), protector)
}

func newNativeProtector(root string) (DataProtector, error) {
	return newFileProtector(filepath.Join(root, "vault.key"))
}

type fileProtector struct {
	key []byte
}

func newFileProtector(keyPath string) (*fileProtector, error) {
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		return nil, fmt.Errorf("create vault key directory: %w", err)
	}
	key, err := os.ReadFile(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		key = make([]byte, fileVaultKeyBytes)
		if _, err := rand.Read(key); err != nil {
			return nil, fmt.Errorf("generate vault key: %w", err)
		}
		if err := os.WriteFile(keyPath, key, 0o600); err != nil {
			return nil, fmt.Errorf("write vault key: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("read vault key: %w", err)
	}
	if len(key) != fileVaultKeyBytes {
		return nil, errors.New("vault key must be 32 bytes")
	}
	return &fileProtector{key: key}, nil
}

func (p *fileProtector) Protect(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(p.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, fileVaultNonceBytes)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	return append(nonce, gcm.Seal(nil, nonce, plaintext, nil)...), nil
}

func (p *fileProtector) Unprotect(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < fileVaultNonceBytes {
		return nil, errors.New("ciphertext is too short")
	}
	block, err := aes.NewCipher(p.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce, payload := ciphertext[:fileVaultNonceBytes], ciphertext[fileVaultNonceBytes:]
	plain, err := gcm.Open(nil, nonce, payload, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt vault data: %w", err)
	}
	return plain, nil
}

type fileCredentialStore struct {
	path      string
	protector DataProtector
	mu        sync.Mutex
}

func newFileCredentialStore(path string, protector DataProtector) (*fileCredentialStore, error) {
	if protector == nil {
		return nil, errors.New("data protector is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve credential store path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		return nil, fmt.Errorf("create credential store directory: %w", err)
	}
	return &fileCredentialStore{path: absolute, protector: protector}, nil
}

func (s *fileCredentialStore) Get(profileID string) (Credential, error) {
	if err := validateProfileID(profileID); err != nil {
		return Credential{}, err
	}
	values, err := s.load()
	if err != nil {
		return Credential{}, err
	}
	value, ok := values[profileID]
	if !ok {
		return Credential{}, ErrNotFound
	}
	return value, nil
}

func (s *fileCredentialStore) Put(profileID string, credential Credential) error {
	if err := validateProfileID(profileID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	values, err := s.loadLocked()
	if err != nil {
		return err
	}
	values[profileID] = credential
	return s.saveLocked(values)
}

func (s *fileCredentialStore) Delete(profileID string) error {
	if err := validateProfileID(profileID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	values, err := s.loadLocked()
	if err != nil {
		return err
	}
	delete(values, profileID)
	return s.saveLocked(values)
}

func (s *fileCredentialStore) load() (map[string]Credential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *fileCredentialStore) loadLocked() (map[string]Credential, error) {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) || len(raw) == 0 {
		return map[string]Credential{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read credential store: %w", err)
	}
	plain, err := s.protector.Unprotect(raw)
	if err != nil {
		return nil, err
	}
	defer clearBytes(plain)
	values := map[string]Credential{}
	if err := json.Unmarshal(plain, &values); err != nil {
		return nil, fmt.Errorf("decode credential store: %w", err)
	}
	return values, nil
}

func (s *fileCredentialStore) saveLocked(values map[string]Credential) error {
	plain, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("encode credential store: %w", err)
	}
	defer clearBytes(plain)
	ciphertext, err := s.protector.Protect(plain)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, ciphertext, 0o600); err != nil {
		return fmt.Errorf("write credential store: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace credential store: %w", err)
	}
	return nil
}
