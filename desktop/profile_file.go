package desktop

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func writeProfileFile(path string, data []byte) error {
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create profile metadata directory: %w", err)
	}
	temporary, err := os.CreateTemp(parent, ".profiles-*.tmp")
	if err != nil {
		return fmt.Errorf("create profile metadata update: %w", err)
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
		return fmt.Errorf("secure profile metadata update: %w", err)
	}
	if _, err := io.WriteString(temporary, string(data)); err != nil {
		return fmt.Errorf("write profile metadata update: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync profile metadata update: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close profile metadata update: %w", err)
	}
	if err := replaceProfileFile(temporaryName, path); err != nil {
		return fmt.Errorf("replace profile metadata: %w", err)
	}
	committed = true
	return nil
}
