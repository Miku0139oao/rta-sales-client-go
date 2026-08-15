package desktop

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const (
	profileFileVersion  = 1
	maximumProfileBytes = 1 << 20
)

type profileRecord struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Enabled     bool   `json:"enabled"`
	Priority    int    `json:"priority"`
}

type profileDocument struct {
	Version  int             `json:"version"`
	Profiles []profileRecord `json:"profiles"`
}

type profileRepository interface {
	List() ([]profileRecord, error)
	Replace([]profileRecord) error
}

// FileProfileRepository stores non-sensitive profile metadata only.
type FileProfileRepository struct {
	path string
	mu   sync.Mutex
}

func NewFileProfileRepository(root string) (*FileProfileRepository, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("profile root is required")
	}
	absolute, err := filepath.Abs(filepath.Join(root, "profiles.json"))
	if err != nil {
		return nil, fmt.Errorf("resolve profile metadata path: %w", err)
	}
	return &FileProfileRepository{path: absolute}, nil
}

func (r *FileProfileRepository) List() ([]profileRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.load()
}

func (r *FileProfileRepository) Replace(profiles []profileRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	profiles = append([]profileRecord(nil), profiles...)
	sortProfiles(profiles)
	if err := validateProfiles(profiles); err != nil {
		return err
	}
	for index := range profiles {
		profiles[index].Priority = index
	}
	document := profileDocument{Version: profileFileVersion, Profiles: profiles}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encode profile metadata: %w", err)
	}
	data = append(data, '\n')
	return writeProfileFile(r.path, data)
}

func (r *FileProfileRepository) load() ([]profileRecord, error) {
	file, err := os.Open(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return []profileRecord{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open profile metadata: %w", err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect profile metadata: %w", err)
	}
	if info.Size() < 0 || info.Size() > maximumProfileBytes {
		return nil, errors.New("profile metadata exceeds 1 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(file, maximumProfileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read profile metadata: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var document profileDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode profile metadata: %w", err)
	}
	if document.Version != profileFileVersion {
		return nil, fmt.Errorf("unsupported profile metadata version %d", document.Version)
	}
	if err := validateProfiles(document.Profiles); err != nil {
		return nil, err
	}
	profiles := append([]profileRecord(nil), document.Profiles...)
	sortProfiles(profiles)
	for index := range profiles {
		profiles[index].Priority = index
	}
	return profiles, nil
}

func validateProfiles(profiles []profileRecord) error {
	seen := make(map[string]struct{}, len(profiles))
	for _, profile := range profiles {
		if !validProfileID(profile.ID) {
			return errors.New("profile metadata contains an invalid identifier")
		}
		if strings.TrimSpace(profile.DisplayName) == "" || len([]rune(profile.DisplayName)) > 80 {
			return errors.New("profile metadata contains an invalid display name")
		}
		if profile.Priority < 0 {
			return errors.New("profile metadata contains an invalid priority")
		}
		if _, exists := seen[profile.ID]; exists {
			return errors.New("profile metadata contains a duplicate identifier")
		}
		seen[profile.ID] = struct{}{}
	}
	return nil
}

func sortProfiles(profiles []profileRecord) {
	sort.SliceStable(profiles, func(left, right int) bool {
		if profiles[left].Priority == profiles[right].Priority {
			return profiles[left].ID < profiles[right].ID
		}
		return profiles[left].Priority < profiles[right].Priority
	})
}

func validProfileID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, character := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if character != '-' {
				return false
			}
			continue
		}
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}
