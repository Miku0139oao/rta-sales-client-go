package desktop

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

type HistoryReport struct {
	Version    int                         `json:"version"`
	ID         string                      `json:"id"`
	SavedAt    string                      `json:"savedAt"`
	Name       string                      `json:"name"`
	From       string                      `json:"from"`
	To         string                      `json:"to"`
	Scope      string                      `json:"scope"`
	ScopeLabel string                      `json:"scopeLabel"`
	Complete   bool                        `json:"complete"`
	Totals     SalesAnalysisTotals         `json:"totals"`
	Stores     []SalesAnalysisStoreSummary `json:"stores"`
}

var historyID = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

func (a *App) historyRoot() (string, error) {
	if a.reportCache == nil {
		return "", errors.New("local history unavailable / 本機歷史資料儲存不可用")
	}
	return filepath.Join(a.reportCache.root, "report-history-v1"), nil
}

func (a *App) ListReportHistory() ([]HistoryReport, error) {
	release, err := a.admitWork()
	if err != nil {
		return nil, err
	}
	defer release()
	root, err := a.historyRoot()
	if err != nil {
		return nil, err
	}
	a.reportCache.writeMu.Lock()
	defer a.reportCache.writeMu.Unlock()
	return readReportHistory(root)
}

func readReportHistory(root string) ([]HistoryReport, error) {
	files, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return []HistoryReport{}, nil
	}
	if err != nil {
		return nil, err
	}
	reports := []HistoryReport{}
	for _, file := range files {
		if file.IsDir() || file.Type()&os.ModeSymlink != 0 || filepath.Ext(file.Name()) != ".json" {
			continue
		}
		info, err := file.Info()
		if err != nil {
			return nil, err
		}
		if info.Size() > 2<<20 {
			return nil, errors.New("history file exceeds limit / 歷史檔案超出大小限制")
		}
		data, err := os.ReadFile(filepath.Join(root, file.Name()))
		if err != nil {
			return nil, err
		}
		var record HistoryReport
		if err := json.Unmarshal(data, &record); err != nil || record.Version != 1 || !historyID.MatchString(record.ID) || file.Name() != record.ID+".json" {
			return nil, errors.New("history file invalid; preserve it for recovery / 歷史檔案損壞，已保留原檔")
		}
		reports = append(reports, record)
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].SavedAt > reports[j].SavedAt })
	return reports, nil
}

func (a *App) SaveReportHistory(record HistoryReport) error {
	release, err := a.admitWork()
	if err != nil {
		return err
	}
	defer release()
	if record.Version != 1 || !historyID.MatchString(record.ID) || len(record.Name) > 240 || record.Name == "" || len(record.Scope) > 256<<10 || len(record.ScopeLabel) > 8192 || len(record.Stores) > 2000 || !record.Complete {
		return errors.New("invalid finished history report / 只能儲存完整報表")
	}
	if _, err := time.Parse(time.RFC3339, record.SavedAt); err != nil {
		return err
	}
	if _, err := time.Parse("2006-01-02", record.From); err != nil {
		return err
	}
	if _, err := time.Parse("2006-01-02", record.To); err != nil {
		return err
	}
	if record.From > record.To {
		return errors.New("invalid history dates")
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if len(data) > 2<<20 {
		return errors.New("history too large / 歷史報表過大")
	}
	root, err := a.historyRoot()
	if err != nil {
		return err
	}
	a.reportCache.writeMu.Lock()
	defer a.reportCache.writeMu.Unlock()
	existing, err := readReportHistory(root)
	if err != nil {
		return err
	}
	if len(existing) >= 20 {
		return errors.New("history limit reached; delete an old report first / 已達 20 份上限，請先刪除舊報表")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	target := filepath.Join(root, record.ID+".json")
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		return errors.New("history already exists / 歷史報表已存在")
	}
	return writeReplaceFile(target, data, ".history-*", "history")
}

func (a *App) DeleteReportHistory(id string) error {
	release, err := a.admitWork()
	if err != nil {
		return err
	}
	defer release()
	if !historyID.MatchString(id) {
		return errors.New("invalid history ID")
	}
	root, err := a.historyRoot()
	if err != nil {
		return err
	}
	a.reportCache.writeMu.Lock()
	defer a.reportCache.writeMu.Unlock()
	err = os.Remove(filepath.Join(root, id+".json"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
