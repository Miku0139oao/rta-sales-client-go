package desktop

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"errors"
	"path/filepath"
	"strings"
)

// WriteReportBundle writes a bounded, validated report archive without replacing files.
func (a *App) WriteReportBundle(request SalesAnalysisPDFWriteRequest) (string, error) {
	release, err := a.admitWork()
	if err != nil {
		return "", err
	}
	defer release()
	directory, err := validPDFDirectory(request.Directory)
	if err != nil {
		return "", err
	}
	name := request.Filename
	if len(name) > 180 || name == "" || filepath.Base(name) != name || strings.ContainsAny(name, `/\:<>"|?*`) || !strings.HasSuffix(name, ".zip") {
		return "", errors.New("invalid report archive filename")
	}
	if len(request.DataBase64) > base64.StdEncoding.EncodedLen(200<<20) {
		return "", errors.New("report archive exceeds 200 MiB")
	}
	data, err := base64.StdEncoding.DecodeString(request.DataBase64)
	if err != nil {
		return "", err
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", errors.New("invalid report archive")
	}
	if len(archive.File) == 0 || len(archive.File) > 4000 {
		return "", errors.New("invalid report archive entries")
	}
	for _, file := range archive.File {
		if file.Name == "" || filepath.Base(file.Name) != file.Name || strings.ContainsAny(file.Name, `/\:`) {
			return "", errors.New("invalid report archive entry")
		}
		switch strings.ToLower(filepath.Ext(file.Name)) {
		case ".pdf", ".xlsx", ".md", ".json":
		default:
			return "", errors.New("unsupported report archive entry")
		}
	}
	return writeUniquePDF(directory, name, data)
}
