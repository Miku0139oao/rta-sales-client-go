package desktop

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxSalesAnalysisPDFBytes = 64 << 20

// ChooseSalesAnalysisPDFDirectory opens one native directory picker before
// the frontend renders a separate PDF for each successful store.
func (a *App) ChooseSalesAnalysisPDFDirectory() (string, error) {
	directory, err := a.dialogs.OpenDirectory(a.appContext(), fileDialogOptions{
		Title: "Export store PDF reports / 匯出門店 PDF 報告",
	})
	if err != nil || strings.TrimSpace(directory) == "" {
		return "", err
	}
	return validPDFDirectory(directory)
}

// WriteSalesAnalysisPDF stores one frontend-rendered PDF without overwriting
// an existing report. The native layer owns path validation and file writes.
func (a *App) WriteSalesAnalysisPDF(request SalesAnalysisPDFWriteRequest) (string, error) {
	directory, err := validPDFDirectory(request.Directory)
	if err != nil {
		return "", err
	}
	filename, err := validPDFFilename(request.Filename)
	if err != nil {
		return "", err
	}
	if len(request.DataBase64) > base64.StdEncoding.EncodedLen(maxSalesAnalysisPDFBytes) {
		return "", errors.New("PDF report is too large")
	}
	data, err := base64.StdEncoding.DecodeString(request.DataBase64)
	if err != nil {
		return "", fmt.Errorf("decode PDF report: %w", err)
	}
	if len(data) == 0 || len(data) > maxSalesAnalysisPDFBytes || !bytes.HasPrefix(data, []byte("%PDF-")) {
		return "", errors.New("generated report is not a valid PDF")
	}
	return writeUniquePDF(directory, filename, data)
}

func validPDFDirectory(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("PDF output directory is required")
	}
	directory, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve PDF output directory: %w", err)
	}
	info, err := os.Stat(directory)
	if err != nil {
		return "", fmt.Errorf("open PDF output directory: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("PDF output path is not a directory")
	}
	return directory, nil
}

func validPDFFilename(value string) (string, error) {
	filename := strings.TrimSpace(value)
	if filename == "" || filepath.Base(filename) != filename || !strings.EqualFold(filepath.Ext(filename), ".pdf") {
		return "", errors.New("invalid PDF filename")
	}
	return filename, nil
}

func writeUniquePDF(directory, name string, data []byte) (string, error) {
	extension := filepath.Ext(name)
	base := strings.TrimSuffix(name, extension)
	for suffix := 0; suffix < 1000; suffix++ {
		candidateName := name
		if suffix > 0 {
			candidateName = fmt.Sprintf("%s-%d%s", base, suffix+1, extension)
		}
		candidate := filepath.Join(directory, candidateName)
		file, err := os.OpenFile(candidate, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("create PDF %s: %w", candidateName, err)
		}
		_, writeErr := file.Write(data)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			_ = os.Remove(candidate)
			if writeErr != nil {
				return "", fmt.Errorf("write PDF %s: %w", candidateName, writeErr)
			}
			return "", fmt.Errorf("close PDF %s: %w", candidateName, closeErr)
		}
		return candidate, nil
	}
	return "", errors.New("too many existing PDF files with the same name")
}
