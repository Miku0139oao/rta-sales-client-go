//go:build windows && native_smoke && portable_update_smoke

// Package smokefixture is absent from production binaries.
package smokefixture

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Manifest struct {
	Root    string `json:"root"`
	Nonce   string `json:"nonce"`
	Port    int    `json:"port"`
	Version string `json:"version"`
}

func Load() (Manifest, error) {
	var m Manifest
	path := os.Getenv("RTA_PORTABLE_SMOKE_MANIFEST")
	b, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	if err = json.Unmarshal(b, &m); err != nil {
		return m, err
	}
	if !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(m.Nonce) || m.Port < 1024 || m.Port > 65535 {
		return m, errors.New("invalid smoke identity")
	}
	root, err := filepath.EvalSymlinks(m.Root)
	if err != nil || !filepath.IsAbs(m.Root) || !strings.EqualFold(root, filepath.Clean(m.Root)) {
		return m, errors.New("noncanonical smoke root")
	}
	if !strings.EqualFold(filepath.Clean(path), filepath.Join(root, "manifest.json")) {
		return m, errors.New("manifest outside sandbox")
	}
	marker, err := os.ReadFile(filepath.Join(root, "nonce"))
	if err != nil || string(marker) != m.Nonce {
		return m, errors.New("sandbox nonce mismatch")
	}
	return m, nil
}

func (m Manifest) File(name string) (string, error) {
	path := filepath.Join(m.Root, name)
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(m.Root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errors.New("fixture path escaped sandbox")
	}
	return resolved, nil
}
