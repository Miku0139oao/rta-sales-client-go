package desktop

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func writeProfileFile(path string, data []byte) error {
	return writeReplaceFile(path, data, ".profiles-*.tmp", "profile metadata")
}

func writeManCodeFile(path string, data []byte) error {
	return writeReplaceFile(path, data, ".mancodes-*.tmp", "mancode catalog")
}

func writeReplaceFile(path string, data []byte, tempPattern, label string) error {
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create %s directory: %w", label, err)
	}
	temporary, err := os.CreateTemp(parent, tempPattern)
	if err != nil {
		return fmt.Errorf("create %s update: %w", label, err)
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
		return fmt.Errorf("secure %s update: %w", label, err)
	}
	if _, err := io.WriteString(temporary, string(data)); err != nil {
		return fmt.Errorf("write %s update: %w", label, err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync %s update: %w", label, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close %s update: %w", label, err)
	}
	if err := replaceProfileFile(temporaryName, path); err != nil {
		return fmt.Errorf("replace %s: %w", label, err)
	}
	committed = true
	return nil
}
