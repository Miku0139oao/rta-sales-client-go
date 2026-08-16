package desktop

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ManCodeCatalogTransferResult is returned after a native catalog export or
// import. An empty dialog selection is reported as Cancelled and is a no-op.
type ManCodeCatalogTransferResult struct {
	Cancelled bool           `json:"cancelled"`
	Path      string         `json:"path,omitempty"`
	Groups    []ManCodeGroup `json:"groups,omitempty"`
}

func writeManCodeTransferFile(path string, data []byte) error {
	return writeReplaceFile(path, data, ".itemcodes-*.tmp", "item code catalog")
}

func (a *App) ExportManCodeCatalog() (ManCodeCatalogTransferResult, error) {
	a.manCodeMu.Lock()
	groups, err := a.mancodes.List()
	if err != nil {
		a.manCodeMu.Unlock()
		return ManCodeCatalogTransferResult{}, err
	}
	snapshot := cloneManCodeGroups(groups)
	data, err := encodeManCodeCatalog(snapshot)
	a.manCodeMu.Unlock()
	if err != nil {
		return ManCodeCatalogTransferResult{}, err
	}
	outputPath, err := a.dialogs.SaveFile(a.appContext(), fileDialogOptions{
		Title:           "Export item codes / 匯出商品代碼",
		DefaultFilename: "item-codes.json",
		Filters:         []fileDialogFilter{{DisplayName: "Item code catalog (*.json)", Pattern: "*.json"}},
	})
	if err != nil {
		return ManCodeCatalogTransferResult{}, err
	}
	if strings.TrimSpace(outputPath) == "" {
		return ManCodeCatalogTransferResult{Cancelled: true}, nil
	}
	outputPath, err = validManCodeTransferPath(outputPath)
	if err != nil {
		return ManCodeCatalogTransferResult{}, err
	}
	if err := writeManCodeTransferFile(outputPath, data); err != nil {
		return ManCodeCatalogTransferResult{}, err
	}
	return ManCodeCatalogTransferResult{Path: outputPath, Groups: snapshot}, nil
}

func (a *App) ImportManCodeCatalog() (ManCodeCatalogTransferResult, error) {
	inputPath, err := a.dialogs.OpenFile(a.appContext(), fileDialogOptions{
		Title:   "Import item codes / 匯入商品代碼",
		Filters: []fileDialogFilter{{DisplayName: "Item code catalog (*.json)", Pattern: "*.json"}},
	})
	if err != nil {
		return ManCodeCatalogTransferResult{}, err
	}
	if strings.TrimSpace(inputPath) == "" {
		return ManCodeCatalogTransferResult{Cancelled: true}, nil
	}
	groups, err := readManCodeTransferFile(inputPath)
	if err != nil {
		return ManCodeCatalogTransferResult{}, err
	}
	a.manCodeMu.Lock()
	defer a.manCodeMu.Unlock()
	if err := a.mancodes.Replace(groups); err != nil {
		return ManCodeCatalogTransferResult{}, err
	}
	return ManCodeCatalogTransferResult{Path: inputPath, Groups: cloneManCodeGroups(groups)}, nil
}

func validManCodeTransferPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("mancode catalog path is required")
	}
	if filepath.Ext(path) == "" {
		path += ".json"
	}
	if !strings.EqualFold(filepath.Ext(path), ".json") {
		return "", errors.New("mancode catalog path must use the .json extension")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve mancode catalog path: %w", err)
	}
	if info, err := os.Stat(absolute); err == nil && info.IsDir() {
		return "", errors.New("mancode catalog path is a directory")
	}
	return absolute, nil
}

func readManCodeTransferFile(path string) ([]ManCodeGroup, error) {
	path, err := validManCodeTransferPath(path)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open mancode catalog: %w", err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect mancode catalog: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("mancode catalog path must be a regular file")
	}
	if info.Size() < 0 || info.Size() > maximumManCodeBytes {
		return nil, errors.New("mancode catalog exceeds 1 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(file, maximumManCodeBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read mancode catalog: %w", err)
	}
	if int64(len(data)) > maximumManCodeBytes {
		return nil, errors.New("mancode catalog exceeds 1 MiB")
	}
	return decodeManCodeCatalog(data)
}
