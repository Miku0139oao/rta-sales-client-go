package desktop

import (
	"fmt"
)

type pathLauncher interface {
	Open(path string) error
	Reveal(path string) error
}

type osPathLauncher struct{}

func (osPathLauncher) Open(path string) error {
	return openPath(path)
}

func (osPathLauncher) Reveal(path string) error {
	return revealPath(path)
}

func (a *App) pathLauncher() pathLauncher {
	if a != nil && a.launcher != nil {
		return a.launcher
	}
	return osPathLauncher{}
}

// OpenSavedWorkbook opens a previously written .xlsx file with the default
// application. The path must already exist; this never creates or overwrites
// a workbook.
func (a *App) OpenSavedWorkbook(request PathRequest) error {
	path, err := existingWorkbookPath(request.Path)
	if err != nil {
		return err
	}
	if err := a.pathLauncher().Open(path); err != nil {
		return fmt.Errorf("open saved workbook: %w", err)
	}
	return nil
}

// RevealSavedWorkbook selects a previously written .xlsx file in the system
// file manager. The path must already exist.
func (a *App) RevealSavedWorkbook(request PathRequest) error {
	path, err := existingWorkbookPath(request.Path)
	if err != nil {
		return err
	}
	if err := a.pathLauncher().Reveal(path); err != nil {
		return fmt.Errorf("reveal saved workbook: %w", err)
	}
	return nil
}

// OpenSavedFolder opens an existing directory in the system file manager.
func (a *App) OpenSavedFolder(request PathRequest) error {
	path, err := existingDirectoryPath(request.Path)
	if err != nil {
		return err
	}
	if err := a.pathLauncher().Open(path); err != nil {
		return fmt.Errorf("open saved folder: %w", err)
	}
	return nil
}
