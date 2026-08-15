package desktop

import (
	"context"
	"time"

	"github.com/Miku0139oao/rta-sales-client-go/xlsxfill"
)

type engineScanRequest struct {
	InputPath string
	Sheet     string
	From      time.Time
	To        time.Time
}

type engineAnalyzeRequest struct {
	InputPath               string
	Sheet                   string
	From                    time.Time
	To                      time.Time
	Overwrite               bool
	MaxJobs                 int
	Concurrency             int
	AllowedBusinessStoreIDs []string
	Mapper                  xlsxfill.StoreMapper
	Progress                func(engineProgress)
}

type engineProgress struct {
	Completed int
	Total     int
	Date      string
	StoreID   string
	Profile   string
	Attempt   int
	Status    string
}

type enginePlan struct {
	Handle                any
	PlanID                string
	InputPath             string
	Complete              bool
	ProblemCount          int
	AggregateProblemCount int
	RetryableCount        int
	ChangedCellCount      int
	Preview               []PreviewRow
}

type engineApplyResult struct {
	Complete         bool
	ProblemCount     int
	ChangedCellCount int
	WroteWorkbook    bool
}

// batchEngine isolates the Wails API from xlsxfill's internal plan type while
// retaining the real plan only in process memory.
type batchEngine interface {
	Scan(engineScanRequest) (WorkbookScan, error)
	Analyze(context.Context, xlsxfill.SalesProvider, engineAnalyzeRequest) (*enginePlan, error)
	RetryFailed(context.Context, *enginePlan, func(engineProgress)) (*enginePlan, error)
	Apply(context.Context, *enginePlan, string, bool) (engineApplyResult, error)
}
