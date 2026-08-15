//go:build !windows

package desktop

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
)

func openPath(path string) error {
	cmd, err := openPathCommand(path)
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open path: %w", err)
	}
	return nil
}

func revealPath(path string) error {
	cmd, err := revealPathCommand(path)
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("reveal path: %w", err)
	}
	return nil
}

func openPathCommand(path string) (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", path), nil
	default:
		return exec.Command("xdg-open", path), nil
	}
}

func revealPathCommand(path string) (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", "-R", path), nil
	default:
		return exec.Command("xdg-open", filepath.Dir(path)), nil
	}
}
