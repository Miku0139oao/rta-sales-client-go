package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakePathLauncher struct {
	opened   string
	revealed string
	err      error
}

func (f *fakePathLauncher) Open(path string) error {
	f.opened = path
	return f.err
}

func (f *fakePathLauncher) Reveal(path string) error {
	f.revealed = path
	return f.err
}

func TestOpenSavedWorkbookRejectsUnsafePaths(t *testing.T) {
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{})
	launcher := new(fakePathLauncher)
	app.launcher = launcher

	if err := app.OpenSavedWorkbook(PathRequest{}); err == nil {
		t.Fatal("expected empty path rejection")
	}
	if err := app.RevealSavedWorkbook(PathRequest{Path: filepath.Join(t.TempDir(), "missing.xlsx")}); err == nil {
		t.Fatal("expected missing file rejection")
	}
	textPath := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(textPath, []byte("not a workbook"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := app.OpenSavedWorkbook(PathRequest{Path: textPath}); err == nil {
		t.Fatal("expected non-xlsx rejection")
	}
	if launcher.opened != "" || launcher.revealed != "" {
		t.Fatalf("unsafe path reached launcher: opened=%q revealed=%q", launcher.opened, launcher.revealed)
	}
}

func TestOpenAndRevealSavedWorkbookUsesExistingFile(t *testing.T) {
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{})
	launcher := new(fakePathLauncher)
	app.launcher = launcher
	path := testWorkbook(t)

	if err := app.OpenSavedWorkbook(PathRequest{Path: path}); err != nil {
		t.Fatal(err)
	}
	if err := app.RevealSavedWorkbook(PathRequest{Path: path}); err != nil {
		t.Fatal(err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(launcher.opened, absolute) || !strings.EqualFold(launcher.revealed, absolute) {
		t.Fatalf("opened=%q revealed=%q, want %q", launcher.opened, launcher.revealed, absolute)
	}
}

func TestOpenSavedFolderOpensExistingDirectory(t *testing.T) {
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{})
	launcher := new(fakePathLauncher)
	app.launcher = launcher
	directory := t.TempDir()

	if err := app.OpenSavedFolder(PathRequest{Path: directory}); err != nil {
		t.Fatal(err)
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(launcher.opened, absolute) {
		t.Fatalf("opened=%q, want %q", launcher.opened, absolute)
	}
}

func TestOpenSavedFolderRejectsUnsafePaths(t *testing.T) {
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{})
	launcher := new(fakePathLauncher)
	app.launcher = launcher

	if err := app.OpenSavedFolder(PathRequest{}); err == nil {
		t.Fatal("expected empty path rejection")
	}
	if err := app.OpenSavedFolder(PathRequest{Path: filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Fatal("expected missing directory rejection")
	}
	filePath := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(filePath, []byte("not a folder"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := app.OpenSavedFolder(PathRequest{Path: filePath}); err == nil {
		t.Fatal("expected file rejection")
	}
	if launcher.opened != "" {
		t.Fatalf("unsafe path reached launcher: opened=%q", launcher.opened)
	}
}

func TestOpenSavedWorkbookSurfacesLauncherError(t *testing.T) {
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{})
	app.launcher = &fakePathLauncher{err: errors.New("association missing")}
	if err := app.OpenSavedWorkbook(PathRequest{Path: testWorkbook(t)}); err == nil {
		t.Fatal("expected launcher error")
	}
}
