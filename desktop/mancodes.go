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
	"unicode"
)

const (
	manCodeFileVersion  = 1
	maximumManCodeBytes = 1 << 20
	manCodeSortPadWidth = 12
)

type manCodeDocument struct {
	Version int            `json:"version"`
	Groups  []ManCodeGroup `json:"groups"`
}

type manCodeRepository interface {
	List() ([]ManCodeGroup, error)
	Replace([]ManCodeGroup) error
}

// FileManCodeRepository stores the local ItemCode catalog next to profiles.json.
type FileManCodeRepository struct {
	path string
	mu   sync.Mutex
}

func NewFileManCodeRepository(root string) (*FileManCodeRepository, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("mancode root is required")
	}
	absolute, err := filepath.Abs(filepath.Join(root, "mancodes.json"))
	if err != nil {
		return nil, fmt.Errorf("resolve mancode catalog path: %w", err)
	}
	return &FileManCodeRepository{path: absolute}, nil
}

func (r *FileManCodeRepository) List() ([]ManCodeGroup, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.load()
}

func (r *FileManCodeRepository) Replace(groups []ManCodeGroup) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	normalized, err := normalizeManCodeGroups(groups)
	if err != nil {
		return err
	}
	document := manCodeDocument{Version: manCodeFileVersion, Groups: normalized}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encode mancode catalog: %w", err)
	}
	data = append(data, '\n')
	return writeManCodeFile(r.path, data)
}

func (r *FileManCodeRepository) load() ([]ManCodeGroup, error) {
	file, err := os.Open(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return []ManCodeGroup{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open mancode catalog: %w", err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect mancode catalog: %w", err)
	}
	if info.Size() < 0 || info.Size() > maximumManCodeBytes {
		return nil, errors.New("mancode catalog exceeds 1 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(file, maximumManCodeBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read mancode catalog: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var document manCodeDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode mancode catalog: %w", err)
	}
	if document.Version != manCodeFileVersion {
		return nil, fmt.Errorf("unsupported mancode catalog version %d", document.Version)
	}
	return normalizeManCodeGroups(document.Groups)
}

func (a *App) ListManCodeGroups() ([]ManCodeGroup, error) {
	a.manCodeMu.Lock()
	defer a.manCodeMu.Unlock()
	return a.mancodes.List()
}

func (a *App) SaveManCodeGroup(request SaveManCodeGroupRequest) (ManCodeGroup, error) {
	name := strings.TrimSpace(request.Name)
	if name == "" {
		return ManCodeGroup{}, errors.New("group name is required")
	}
	a.manCodeMu.Lock()
	defer a.manCodeMu.Unlock()
	groups, err := a.mancodes.List()
	if err != nil {
		return ManCodeGroup{}, err
	}
	index := -1
	for candidate := range groups {
		if groups[candidate].ID == request.ID {
			index = candidate
			break
		}
	}
	isNew := strings.TrimSpace(request.ID) == ""
	if isNew {
		request.ID, err = newUUID()
		if err != nil {
			return ManCodeGroup{}, err
		}
		index = len(groups)
		groups = append(groups, ManCodeGroup{ID: request.ID})
	} else if index < 0 || !validProfileID(request.ID) {
		return ManCodeGroup{}, errors.New("mancode group does not exist")
	}
	if manCodeGroupNameTaken(groups, request.ID, name) {
		return ManCodeGroup{}, errors.New("group name already exists")
	}
	groups[index].Name = name
	if isNew || request.Codes != nil || request.Raw != "" {
		groups[index].Codes = collectManCodes(request.Raw, request.Codes)
	}
	if err := a.mancodes.Replace(groups); err != nil {
		return ManCodeGroup{}, err
	}
	return cloneManCodeGroup(groups[index]), nil
}

func (a *App) DeleteManCodeGroup(id string) error {
	a.manCodeMu.Lock()
	defer a.manCodeMu.Unlock()
	groups, err := a.mancodes.List()
	if err != nil {
		return err
	}
	index := -1
	for candidate := range groups {
		if groups[candidate].ID == id {
			index = candidate
			break
		}
	}
	if index < 0 {
		return errors.New("mancode group does not exist")
	}
	groups = append(groups[:index], groups[index+1:]...)
	return a.mancodes.Replace(groups)
}

func (a *App) ReplaceManCodeGroupCodes(request ReplaceManCodeGroupCodesRequest) (ManCodeGroup, error) {
	a.manCodeMu.Lock()
	defer a.manCodeMu.Unlock()
	groups, err := a.mancodes.List()
	if err != nil {
		return ManCodeGroup{}, err
	}
	index := -1
	for candidate := range groups {
		if groups[candidate].ID == request.ID {
			index = candidate
			break
		}
	}
	if index < 0 {
		return ManCodeGroup{}, errors.New("mancode group does not exist")
	}
	groups[index].Codes = collectManCodes(request.Raw, request.Codes)
	if err := a.mancodes.Replace(groups); err != nil {
		return ManCodeGroup{}, err
	}
	return cloneManCodeGroup(groups[index]), nil
}

func collectManCodes(raw string, codes []string) []string {
	values := make([]string, 0, len(codes)+1)
	values = append(values, raw)
	values = append(values, codes...)
	return normalizeManCodes(values...)
}

func normalizeManCodes(values ...string) []string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, splitManCodes(value)...)
	}
	seen := make(map[string]struct{}, len(parts))
	unique := make([]string, 0, len(parts))
	for _, code := range parts {
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		unique = append(unique, code)
	}
	sort.SliceStable(unique, func(left, right int) bool {
		leftKey, rightKey := manCodeSortKey(unique[left]), manCodeSortKey(unique[right])
		if leftKey == rightKey {
			return unique[left] < unique[right]
		}
		return leftKey < rightKey
	})
	return unique
}

func splitManCodes(raw string) []string {
	fields := strings.FieldsFunc(raw, func(character rune) bool {
		return unicode.IsSpace(character) || character == ',' || character == '，'
	})
	codes := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		codes = append(codes, field)
	}
	return codes
}

func manCodeSortKey(code string) string {
	if manCodeAllDigits(code) {
		if len(code) >= manCodeSortPadWidth {
			return code
		}
		return strings.Repeat("0", manCodeSortPadWidth-len(code)) + code
	}
	return strings.ToLower(code)
}

func manCodeAllDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func normalizeManCodeGroups(groups []ManCodeGroup) ([]ManCodeGroup, error) {
	normalized := make([]ManCodeGroup, 0, len(groups))
	for _, group := range groups {
		group.Name = strings.TrimSpace(group.Name)
		group.Codes = normalizeManCodes(group.Codes...)
		normalized = append(normalized, group)
	}
	if err := validateManCodeGroups(normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

func validateManCodeGroups(groups []ManCodeGroup) error {
	seenIDs := make(map[string]struct{}, len(groups))
	seenNames := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		if !validProfileID(group.ID) {
			return errors.New("mancode catalog contains an invalid identifier")
		}
		if group.Name == "" {
			return errors.New("mancode catalog contains an invalid group name")
		}
		if _, exists := seenIDs[group.ID]; exists {
			return errors.New("mancode catalog contains a duplicate identifier")
		}
		if _, exists := seenNames[group.Name]; exists {
			return errors.New("mancode catalog contains a duplicate group name")
		}
		seenIDs[group.ID] = struct{}{}
		seenNames[group.Name] = struct{}{}
	}
	return nil
}

func manCodeGroupNameTaken(groups []ManCodeGroup, id, name string) bool {
	for _, group := range groups {
		if group.ID != id && group.Name == name {
			return true
		}
	}
	return false
}

func cloneManCodeGroup(group ManCodeGroup) ManCodeGroup {
	group.Codes = append([]string(nil), group.Codes...)
	if group.Codes == nil {
		group.Codes = []string{}
	}
	return group
}
