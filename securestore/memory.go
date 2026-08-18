package securestore

import (
	"errors"
	"sync"
)

// MemoryCredentialStore is a concurrency-safe test fake. It never touches an
// OS vault and should not be used for production persistence.
type MemoryCredentialStore struct {
	mu     sync.RWMutex
	values map[string]Credential
}

func NewMemoryCredentialStore() *MemoryCredentialStore {
	return &MemoryCredentialStore{values: make(map[string]Credential)}
}

func (s *MemoryCredentialStore) Get(profileID string) (Credential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.values[profileID]
	if !ok {
		return Credential{}, ErrNotFound
	}
	return value, nil
}

func (s *MemoryCredentialStore) Put(profileID string, value Credential) error {
	if err := validateProfileID(profileID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[profileID] = value
	return nil
}

func (s *MemoryCredentialStore) Delete(profileID string) error {
	if err := validateProfileID(profileID); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, profileID)
	return nil
}

func (s *MemoryCredentialStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values = make(map[string]Credential)
}

// MemoryCookieStore is an in-memory implementation of rtasales.CookieStore.
type MemoryCookieStore struct {
	mu   sync.RWMutex
	data []byte
}

func (s *MemoryCookieStore) Load() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]byte(nil), s.data...), nil
}

func (s *MemoryCookieStore) Save(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = append(s.data[:0], data...)
	return nil
}

// MemoryProtector is a deterministic, reversible test fake. It exists only to
// verify encrypted-file wiring and is not cryptographically secure.
type MemoryProtector struct{}

var memoryProtectorHeader = []byte("rta-test-protected-v1\x00")

func (MemoryProtector) Protect(plaintext []byte) ([]byte, error) {
	result := make([]byte, len(memoryProtectorHeader)+len(plaintext))
	copy(result, memoryProtectorHeader)
	for index, value := range plaintext {
		result[len(memoryProtectorHeader)+index] = value ^ 0xa5
	}
	return result, nil
}

func (MemoryProtector) Unprotect(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < len(memoryProtectorHeader) {
		return nil, errors.New("invalid protected test data")
	}
	for index := range memoryProtectorHeader {
		if ciphertext[index] != memoryProtectorHeader[index] {
			return nil, errors.New("invalid protected test data")
		}
	}
	result := make([]byte, len(ciphertext)-len(memoryProtectorHeader))
	for index, value := range ciphertext[len(memoryProtectorHeader):] {
		result[index] = value ^ 0xa5
	}
	return result, nil
}
