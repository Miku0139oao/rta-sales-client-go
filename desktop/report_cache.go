package desktop

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	salesReportCacheFile    = "sales-report.json.gz"
	workbookSessionFile     = "workbook-session.json"
	salesReportCacheVersion = 1
	workbookSessionVersion  = 1
	// Packed article rows for many stores can reach tens of megabytes once
	// decompressed; anything beyond this is treated as corrupt.
	maximumSalesReportBytes     = 256 << 20
	maximumWorkbookSessionBytes = 64 << 10
)

// reportCacheStore persists the last finished sales report and the last
// scanned workbook under the per-user application directory so the operator
// returns to the same screen after restarting the desktop app.
type reportCacheStore struct {
	root    string
	writeMu sync.Mutex
	pending sync.WaitGroup
}

func newReportCacheStore(root string) (*reportCacheStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("report cache root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve report cache root: %w", err)
	}
	return &reportCacheStore{root: absolute}, nil
}

type salesReportDocument struct {
	Version   int                                 `json:"version"`
	SavedAt   time.Time                           `json:"savedAt"`
	ProfileID string                              `json:"profileId,omitempty"`
	Result    SalesAnalysisResult                 `json:"result"`
	Packed    map[string]SalesAnalysisPackedItems `json:"packed"`
}

type workbookSessionDocument struct {
	Version   int       `json:"version"`
	SavedAt   time.Time `json:"savedAt"`
	InputPath string    `json:"inputPath"`
	SheetName string    `json:"sheetName,omitempty"`
}

func (s *reportCacheStore) salesReportPath() string {
	return filepath.Join(s.root, salesReportCacheFile)
}

func (s *reportCacheStore) workbookSessionPath() string {
	return filepath.Join(s.root, workbookSessionFile)
}

func (s *reportCacheStore) saveSalesReport(document salesReportDocument) error {
	document.Version = salesReportCacheVersion
	var buffer bytes.Buffer
	compressor := gzip.NewWriter(&buffer)
	if err := json.NewEncoder(compressor).Encode(document); err != nil {
		return fmt.Errorf("encode sales report cache: %w", err)
	}
	if err := compressor.Close(); err != nil {
		return fmt.Errorf("compress sales report cache: %w", err)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return writeReplaceFile(s.salesReportPath(), buffer.Bytes(), ".sales-report-*.tmp", "sales report cache")
}

// loadSalesReport returns ok=false when no usable cache exists. Corrupt or
// outdated files are removed so a broken cache never blocks the next launch.
func (s *reportCacheStore) loadSalesReport() (salesReportDocument, bool, error) {
	path := s.salesReportPath()
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return salesReportDocument{}, false, nil
		}
		return salesReportDocument{}, false, fmt.Errorf("open sales report cache: %w", err)
	}
	defer func() { _ = file.Close() }()
	document, err := decodeSalesReport(file)
	if err != nil {
		_ = os.Remove(path)
		return salesReportDocument{}, false, nil
	}
	return document, true, nil
}

func decodeSalesReport(reader io.Reader) (salesReportDocument, error) {
	decompressor, err := gzip.NewReader(reader)
	if err != nil {
		return salesReportDocument{}, err
	}
	defer func() { _ = decompressor.Close() }()
	limited := &io.LimitedReader{R: decompressor, N: maximumSalesReportBytes + 1}
	var document salesReportDocument
	if err := json.NewDecoder(limited).Decode(&document); err != nil {
		return salesReportDocument{}, err
	}
	if limited.N <= 0 {
		return salesReportDocument{}, errors.New("sales report cache exceeds the size limit")
	}
	if document.Version != salesReportCacheVersion {
		return salesReportDocument{}, fmt.Errorf("unsupported sales report cache version %d", document.Version)
	}
	if strings.TrimSpace(document.Result.OperationID) == "" || document.Result.Pending {
		return salesReportDocument{}, errors.New("sales report cache is incomplete")
	}
	if document.Packed == nil {
		document.Packed = make(map[string]SalesAnalysisPackedItems)
	}
	return document, nil
}

func (s *reportCacheStore) clearSalesReport() error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := os.Remove(s.salesReportPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove sales report cache: %w", err)
	}
	return nil
}

func (s *reportCacheStore) saveWorkbookSession(inputPath, sheetName string) error {
	document := workbookSessionDocument{
		Version: workbookSessionVersion, SavedAt: time.Now(),
		InputPath: strings.TrimSpace(inputPath), SheetName: strings.TrimSpace(sheetName),
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encode workbook session: %w", err)
	}
	data = append(data, '\n')
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return writeReplaceFile(s.workbookSessionPath(), data, ".workbook-session-*.tmp", "workbook session")
}

func (s *reportCacheStore) loadWorkbookSession() (workbookSessionDocument, bool, error) {
	path := s.workbookSessionPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return workbookSessionDocument{}, false, nil
		}
		return workbookSessionDocument{}, false, fmt.Errorf("read workbook session: %w", err)
	}
	var document workbookSessionDocument
	if len(data) > maximumWorkbookSessionBytes || json.Unmarshal(data, &document) != nil ||
		document.Version != workbookSessionVersion || strings.TrimSpace(document.InputPath) == "" {
		_ = os.Remove(path)
		return workbookSessionDocument{}, false, nil
	}
	return document, true, nil
}

func (s *reportCacheStore) clearWorkbookSession() error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := os.Remove(s.workbookSessionPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove workbook session: %w", err)
	}
	return nil
}

// waitIdle blocks until background writes finish or the timeout elapses.
func (s *reportCacheStore) waitIdle(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		s.pending.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}
