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
	ProfileID          string `json:"profileId"`
	SimulateStoreCount int    `json:"simulateStoreCount,omitempty"`
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
	ProfileID          string `json:"profileId,omitempty"`
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

type PathRequest struct {
	Path string `json:"path"`
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

// SalesAnalysisStore is an authorized, business-facing store available to a
// desktop account profile. Query-only RTA identifiers never leave the client.
type SalesAnalysisStore struct {
	BusinessID string `json:"businessId"`
	Label      string `json:"label"`
	ProfileID  string `json:"profileId,omitempty"`
	Profile    string `json:"profile,omitempty"`
}

type SalesAnalysisRequest struct {
	ProfileID          string                       `json:"profileId"`
	StoreIDs           []string                     `json:"storeIds"`
	From               string                       `json:"from,omitempty"`
	To                 string                       `json:"to,omitempty"`
	Periods            []SalesAnalysisPeriodRequest `json:"periods,omitempty"`
	Concurrency        int                          `json:"concurrency"`
	SimulateStoreCount int                          `json:"simulateStoreCount,omitempty"`
}

type SalesAnalysisPeriodRequest struct {
	Key          string `json:"key"`
	Label        string `json:"label"`
	From         string `json:"from"`
	To           string `json:"to"`
	IncludeTrend bool   `json:"includeTrend"`
}

type SalesAnalysisItem struct {
	StoreID                string  `json:"storeId"`
	StoreLabel             string  `json:"storeLabel"`
	Category1              string  `json:"category1"`
	Category1Code          string  `json:"category1Code,omitempty"`
	Category2              string  `json:"category2"`
	Category2Code          string  `json:"category2Code,omitempty"`
	Category3              string  `json:"category3"`
	Category3Code          string  `json:"category3Code,omitempty"`
	Category4              string  `json:"category4"`
	Category4Code          string  `json:"category4Code,omitempty"`
	Category5              string  `json:"category5"`
	Category5Code          string  `json:"category5Code,omitempty"`
	ArticleCode            string  `json:"articleCode"`
	ArticleName            string  `json:"articleName"`
	BrandName              string  `json:"brandName,omitempty"`
	TransactionCount       float64 `json:"transactionCount"`
	SaleQuantity           float64 `json:"saleQuantity"`
	SaleAmount             float64 `json:"saleAmount"`
	ReturnQuantity         float64 `json:"returnQuantity"`
	ReturnTransactionCount float64 `json:"returnTransactionCount"`
	ReturnAmount           float64 `json:"returnAmount"`
	NetQuantity            float64 `json:"netQuantity"`
	NetSalesAmount         float64 `json:"netSalesAmount"`
}

type SalesAnalysisTotals struct {
	SaleQuantity        float64  `json:"saleQuantity"`
	SaleAmount          float64  `json:"saleAmount"`
	ReturnQuantity      float64  `json:"returnQuantity"`
	ReturnAmount        float64  `json:"returnAmount"`
	NetQuantity         float64  `json:"netQuantity"`
	NetSalesAmount      float64  `json:"netSalesAmount"`
	TrendNetSalesAmount *float64 `json:"trendNetSalesAmount,omitempty"`
	TransactionCount    *float64 `json:"transactionCount,omitempty"`
}

type SalesAnalysisStoreSummary struct {
	BusinessID string              `json:"businessId"`
	Label      string              `json:"label"`
	Totals     SalesAnalysisTotals `json:"totals"`
}

type SalesAnalysisIssue struct {
	PeriodKey  string `json:"periodKey,omitempty"`
	StoreID    string `json:"storeId"`
	StoreLabel string `json:"storeLabel"`
	Message    string `json:"message"`
}

type SalesAnalysisPeriodResult struct {
	Key              string                      `json:"key"`
	Label            string                      `json:"label"`
	From             string                      `json:"from"`
	To               string                      `json:"to"`
	Complete         bool                        `json:"complete"`
	SuccessfulStores int                         `json:"successfulStores"`
	Totals           SalesAnalysisTotals         `json:"totals"`
	Stores           []SalesAnalysisStoreSummary `json:"stores"`
	Items            []SalesAnalysisItem         `json:"items,omitempty"`
	ItemCount        int                         `json:"itemCount"`
	Issues           []SalesAnalysisIssue        `json:"issues,omitempty"`
}

type SalesAnalysisItemsRequest struct {
	OperationID string `json:"operationId"`
	PeriodKey   string `json:"periodKey"`
	StoreID     string `json:"storeId,omitempty"`
}

type SalesAnalysisPackedItems struct {
	PeriodKey string                   `json:"periodKey"`
	Dict      []string                 `json:"dict"`
	Rows      []SalesAnalysisPackedRow `json:"rows"`
}

type SalesAnalysisReportMemoRequest struct {
	OperationID      string   `json:"operationId"`
	StoreID          string   `json:"storeId,omitempty"`
	CategoryLevel    string   `json:"categoryLevel"`
	ExcludeZeroGifts bool     `json:"excludeZeroGifts"`
	ExcludeStamps    bool     `json:"excludeStamps"`
	Mode             string   `json:"mode"`
	Categories       []string `json:"categories,omitempty"`
}

type SalesAnalysisReportMemo struct {
	Periods []SalesAnalysisPeriodMemo `json:"periods"`
}

type SalesAnalysisPeriodMemo struct {
	Key            string                       `json:"key"`
	TopAmount      []SalesAnalysisRankedItem    `json:"topAmount,omitempty"`
	TopQuantity    []SalesAnalysisRankedItem    `json:"topQuantity,omitempty"`
	AmountGroups   []SalesAnalysisCategoryGroup `json:"amountGroups,omitempty"`
	QuantityGroups []SalesAnalysisCategoryGroup `json:"quantityGroups,omitempty"`
	FocusGroups    []SalesAnalysisFocusGroup    `json:"focusGroups,omitempty"`
}

type SalesAnalysisRankedItem struct {
	ID            string  `json:"id"`
	Code          string  `json:"code"`
	Name          string  `json:"name"`
	Brand         string  `json:"brand,omitempty"`
	Amount        float64 `json:"amount"`
	Quantity      float64 `json:"quantity"`
	Category2Code string  `json:"category2Code,omitempty"`
	Category3Code string  `json:"category3Code,omitempty"`
	Category4Code string  `json:"category4Code,omitempty"`
}

type SalesAnalysisCategoryGroup struct {
	ID       string                    `json:"id"`
	Code     string                    `json:"code,omitempty"`
	Name     string                    `json:"name"`
	Amount   float64                   `json:"amount"`
	Quantity float64                   `json:"quantity"`
	Items    []SalesAnalysisRankedItem `json:"items,omitempty"`
}

type SalesAnalysisFocusGroup struct {
	ID       string                      `json:"id"`
	Prefix   string                      `json:"prefix"`
	Sales    []SalesAnalysisFocusProduct `json:"sales,omitempty"`
	Quantity []SalesAnalysisFocusProduct `json:"quantity,omitempty"`
}

type SalesAnalysisFocusProduct struct {
	ID              string  `json:"id"`
	Code            string  `json:"code"`
	Name            string  `json:"name"`
	Brand           string  `json:"brand,omitempty"`
	Amount          float64 `json:"amount"`
	Quantity        float64 `json:"quantity"`
	CurrentAmount   float64 `json:"currentAmount"`
	CurrentQuantity float64 `json:"currentQuantity"`
}

type SalesAnalysisPackedRow struct {
	S  int     `json:"s"`
	Ac int     `json:"ac"`
	An int     `json:"an"`
	Br int     `json:"br,omitempty"`
	C1 int     `json:"c1,omitempty"`
	K1 int     `json:"k1,omitempty"`
	C2 int     `json:"c2,omitempty"`
	K2 int     `json:"k2,omitempty"`
	C3 int     `json:"c3,omitempty"`
	K3 int     `json:"k3,omitempty"`
	C4 int     `json:"c4,omitempty"`
	K4 int     `json:"k4,omitempty"`
	C5 int     `json:"c5,omitempty"`
	K5 int     `json:"k5,omitempty"`
	T  float64 `json:"t,omitempty"`
	Sq float64 `json:"sq,omitempty"`
	Sa float64 `json:"sa,omitempty"`
	Rq float64 `json:"rq,omitempty"`
	Rt float64 `json:"rt,omitempty"`
	Ra float64 `json:"ra,omitempty"`
	Nq float64 `json:"nq,omitempty"`
	Ns float64 `json:"ns,omitempty"`
}

type SalesAnalysisResult struct {
	OperationID      string                      `json:"operationId"`
	From             string                      `json:"from"`
	To               string                      `json:"to"`
	Complete         bool                        `json:"complete"`
	Pending          bool                        `json:"pending,omitempty"`
	SelectedStores   int                         `json:"selectedStores"`
	SuccessfulStores int                         `json:"successfulStores"`
	Totals           SalesAnalysisTotals         `json:"totals"`
	Stores           []SalesAnalysisStoreSummary `json:"stores"`
	Items            []SalesAnalysisItem         `json:"items,omitempty"`
	Issues           []SalesAnalysisIssue        `json:"issues,omitempty"`
	Periods          []SalesAnalysisPeriodResult `json:"periods,omitempty"`
	Weeks            []SalesAnalysisWeek         `json:"weeks,omitempty"`
	QueryDurationMS  int64                       `json:"queryDurationMs"`
}

type SalesAnalysisWeek struct {
	From   string                   `json:"from"`
	To     string                   `json:"to"`
	Stores []SalesAnalysisWeekStore `json:"stores"`
	Totals SalesAnalysisWeekStore   `json:"totals"`
}

type SalesAnalysisWeekStore struct {
	BusinessID         string  `json:"businessId,omitempty"`
	Label              string  `json:"label,omitempty"`
	SalesTW            float64 `json:"salesTw"`
	SalesLW            float64 `json:"salesLw"`
	CustomersTW        float64 `json:"customersTw"`
	CustomersLW        float64 `json:"customersLw"`
	WeekdaySalesTW     float64 `json:"weekdaySalesTw"`
	WeekdaySalesLW     float64 `json:"weekdaySalesLw"`
	WeekendSalesTW     float64 `json:"weekendSalesTw"`
	WeekendSalesLW     float64 `json:"weekendSalesLw"`
	WeekdayCustomersTW float64 `json:"weekdayCustomersTw"`
	WeekdayCustomersLW float64 `json:"weekdayCustomersLw"`
	WeekendCustomersTW float64 `json:"weekendCustomersTw"`
	WeekendCustomersLW float64 `json:"weekendCustomersLw"`
}

type SalesAnalysisPDFWriteRequest struct {
	Directory  string `json:"directory"`
	Filename   string `json:"filename"`
	DataBase64 string `json:"dataBase64"`
}

type SalesAnalysisProgress struct {
	OperationID string `json:"operationId"`
	Current     int    `json:"current"`
	Total       int    `json:"total"`
	StoreID     string `json:"storeId,omitempty"`
	StoreLabel  string `json:"storeLabel,omitempty"`
	PeriodKey   string `json:"periodKey,omitempty"`
	PeriodLabel string `json:"periodLabel,omitempty"`
	Status      string `json:"status,omitempty"`
}
