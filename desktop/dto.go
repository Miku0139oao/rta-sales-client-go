package desktop

// Profile is safe profile metadata returned to the frontend. Account names
// and passwords are intentionally absent.
type Profile struct {
	ID             string `json:"id"`
	DisplayName    string `json:"displayName"`
	Enabled        bool   `json:"enabled"`
	Priority       int    `json:"priority"`
	HasCredentials bool   `json:"hasCredentials"`
}

type ProfileUpsertRequest struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Account     string `json:"account"`
	Password    string `json:"password"`
	Enabled     bool   `json:"enabled"`
}

type ProfileIDRequest struct {
	ProfileID string `json:"profileId"`
}

type TestProfileRequest struct {
	ProfileID string `json:"profileId"`
}

type ProfileTestResult struct {
	ProfileID  string `json:"profileId"`
	StoreCount int    `json:"storeCount"`
	OK         bool   `json:"ok"`
	Success    bool   `json:"success"`
	Message    string `json:"message,omitempty"`
}

type ReorderProfilesRequest struct {
	ProfileIDs []string `json:"profileIds"`
}

type EnableProfileRequest struct {
	ProfileID string `json:"profileId"`
	Enabled   bool   `json:"enabled"`
}

type SaveWorkbookRequest struct {
	InputPath string `json:"inputPath"`
	Date      string `json:"date"`
	From      string `json:"from"`
	To        string `json:"to"`
}

type ScanWorkbookRequest struct {
	InputPath   string `json:"inputPath"`
	MappingPath string `json:"mappingPath"`
	Sheet       string `json:"sheet"`
	SheetName   string `json:"sheetName"`
	Date        string `json:"date"`
	From        string `json:"from"`
	To          string `json:"to"`
}

type SheetSummary struct {
	Name    string `json:"name"`
	DateMin string `json:"dateMin,omitempty"`
	DateMax string `json:"dateMax,omitempty"`
	Rows    int    `json:"rows,omitempty"`
}

type WorkbookScan struct {
	InputPath         string         `json:"inputPath"`
	FileName          string         `json:"fileName"`
	SheetName         string         `json:"sheetName"`
	Sheets            []SheetSummary `json:"sheets"`
	DateMin           string         `json:"dateMin"`
	DateMax           string         `json:"dateMax"`
	Dates             []string       `json:"dates"`
	Rows              int            `json:"rows"`
	Stores            int            `json:"stores"`
	Jobs              int            `json:"jobs"`
	Accounts          int            `json:"accounts"`
	RowCount          int            `json:"rowCount"`
	StoreCount        int            `json:"storeCount"`
	JobCount          int            `json:"jobCount"`
	AvailableProfiles int            `json:"availableProfiles"`
	Warnings          []string       `json:"warnings,omitempty"`
}

type AnalyzeRequest struct {
	InputPath          string `json:"inputPath"`
	MappingPath        string `json:"mappingPath"`
	Sheet              string `json:"sheet"`
	SheetName          string `json:"sheetName"`
	Date               string `json:"date"`
	From               string `json:"from"`
	To                 string `json:"to"`
	Overwrite          bool   `json:"overwrite"`
	AllowPartial       bool   `json:"allowPartial"`
	MaxJobs            int    `json:"maxJobs"`
	MaxQueries         int    `json:"maxQueries"`
	AccountConcurrency int    `json:"accountConcurrency"`
	UseLocalMapping    bool   `json:"useLocalMapping"`
}

type PreviewRow struct {
	ID                 string   `json:"id"`
	Date               string   `json:"date"`
	Row                int      `json:"row"`
	WorkbookStoreID    string   `json:"workbookStoreId"`
	StoreLabel         string   `json:"storeLabel"`
	ProfileDisplayName string   `json:"profileDisplayName"`
	ProfileLabel       string   `json:"profileLabel"`
	CurrentL           string   `json:"currentL"`
	ProposedL          string   `json:"proposedL"`
	CurrentAB          string   `json:"currentAB"`
	ProposedAB         string   `json:"proposedAB"`
	Status             string   `json:"status"`
	Message            string   `json:"message,omitempty"`
	IssueCodes         []string `json:"issueCodes"`
}

type AnalysisIssue struct {
	Row       int    `json:"row,omitempty"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable,omitempty"`
}

type AnalysisResult struct {
	OperationID           string          `json:"operationId"`
	PlanID                string          `json:"planId"`
	Complete              bool            `json:"complete"`
	OverlapCount          int             `json:"overlapCount"`
	ProblemCount          int             `json:"problemCount"`
	AggregateProblemCount int             `json:"aggregateProblemCount"`
	RetryableCount        int             `json:"retryableCount"`
	ChangedCellCount      int             `json:"changedCellCount"`
	Warnings              []string        `json:"warnings"`
	Preview               []PreviewRow    `json:"preview"`
	Rows                  []PreviewRow    `json:"rows"`
	TotalCount            int             `json:"totalCount"`
	ChangeCount           int             `json:"changeCount"`
	UnchangedCount        int             `json:"unchangedCount"`
	IssueCount            int             `json:"issueCount"`
	FailedCount           int             `json:"failedCount"`
	OverlapWarning        string          `json:"overlapWarning,omitempty"`
	Issues                []AnalysisIssue `json:"issues"`
	CanApply              bool            `json:"canApply"`
}

type OperationRequest struct {
	OperationID string `json:"operationId"`
}

type ApplyRequest struct {
	OperationID       string `json:"operationId"`
	InputPath         string `json:"inputPath"`
	OutputPath        string `json:"outputPath"`
	Overwrite         bool   `json:"overwrite"`
	AllowPartial      bool   `json:"allowPartial"`
	KeepIssueOriginal bool   `json:"keepIssueOriginal"`
}

type ApplyResult struct {
	OperationID      string `json:"operationId"`
	OutputPath       string `json:"outputPath"`
	Complete         bool   `json:"complete"`
	ChangedCellCount int    `json:"changedCellCount"`
	ProblemCount     int    `json:"problemCount"`
	WroteWorkbook    bool   `json:"wroteWorkbook"`
	ChangedCells     int    `json:"changedCells"`
	SkippedRows      int    `json:"skippedRows"`
}

// ProgressEvent is the only event payload sent by the backend. Query events may
// identify the latest completed profile, store, and date, but never include
// credentials, cookies, or sales values.
type ProgressEvent struct {
	OperationID string `json:"operationId"`
	Stage       string `json:"stage"`
	Current     int    `json:"current"`
	Total       int    `json:"total"`
	Message     string `json:"message"`
	Date        string `json:"date,omitempty"`
	StoreID     string `json:"storeId,omitempty"`
	Profile     string `json:"profile,omitempty"`
	Attempt     int    `json:"attempt,omitempty"`
	Status      string `json:"status,omitempty"`
}

type RuntimeStatus struct {
	Available bool   `json:"available"`
	Version   string `json:"version,omitempty"`
	Message   string `json:"message"`
}
