package desktop

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	rtasales "github.com/Miku0139oao/rta-sales-client-go"
	"github.com/Miku0139oao/rta-sales-client-go/securestore"
	"github.com/Miku0139oao/rta-sales-client-go/xlsxfill"
)

const progressEventName = "rta:progress"

type appDependencies struct {
	profiles    profileRepository
	credentials securestore.CredentialStore
	cookies     profileCookieStore
	clients     clientFactory
	engine      batchEngine
	dialogs     dialogService
	events      eventSink
	runtime     runtimeChecker
}

// App is the Wails-bound desktop backend. All plans, providers, store routing,
// and workbook preview values remain exclusively in this process.
type App struct {
	profiles    profileRepository
	credentials securestore.CredentialStore
	cookies     profileCookieStore
	clients     clientFactory
	engine      batchEngine
	dialogs     dialogService
	events      eventSink
	runtime     runtimeChecker
	launcher    pathLauncher

	contextMu              sync.RWMutex
	ctx                    context.Context
	profileMu              sync.Mutex
	operationMu            sync.Mutex
	active                 *operationState
	profileMutationRunning bool
	profileTestRunning     bool
	profileTestID          string
	profileTestCancel      context.CancelFunc
	salesAnalysisRunning   bool
	salesAnalysisID        string
	salesAnalysisCancel    context.CancelFunc
	salesResultMu          sync.Mutex
	salesResult            *SalesAnalysisResult
	salesPacked            map[string]SalesAnalysisPackedItems
}

type operationState struct {
	id           string
	plan         *enginePlan
	inputPath    string
	allowPartial bool
	overlapCount int
	warnings     []string
	ownership    map[string]string
	running      bool
	applied      bool
	cancel       context.CancelFunc
}

func newApp(dependencies appDependencies) (*App, error) {
	if dependencies.profiles == nil || dependencies.credentials == nil || dependencies.cookies == nil ||
		dependencies.clients == nil || dependencies.engine == nil || dependencies.dialogs == nil ||
		dependencies.events == nil || dependencies.runtime == nil {
		return nil, errors.New("desktop backend dependencies are incomplete")
	}
	return &App{
		profiles:    dependencies.profiles,
		credentials: dependencies.credentials,
		cookies:     dependencies.cookies,
		clients:     dependencies.clients,
		engine:      dependencies.engine,
		dialogs:     dependencies.dialogs,
		events:      dependencies.events,
		runtime:     dependencies.runtime,
		ctx:         context.Background(),
	}, nil
}

// Start initialises the app context from Wails without exposing a lifecycle
// callback as a frontend-bound App method.
func Start(a *App, ctx context.Context) {
	if a == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	a.contextMu.Lock()
	a.ctx = ctx
	a.contextMu.Unlock()
}

func (a *App) appContext() context.Context {
	a.contextMu.RLock()
	defer a.contextMu.RUnlock()
	return a.ctx
}

func (a *App) CheckRuntime() RuntimeStatus {
	return a.runtime.Check()
}

func (a *App) OpenWorkbook() (string, error) {
	return a.dialogs.OpenFile(a.appContext(), fileDialogOptions{
		Title:   "Open workbook / 開啟活頁簿",
		Filters: []fileDialogFilter{{DisplayName: "Excel workbook (*.xlsx)", Pattern: "*.xlsx"}},
	})
}

func (a *App) OpenMappingFile() (string, error) {
	return a.dialogs.OpenFile(a.appContext(), fileDialogOptions{
		Title:   "Open store mapping / 開啟門店對照檔",
		Filters: []fileDialogFilter{{DisplayName: "Store mapping (*.json;*.csv)", Pattern: "*.json;*.csv"}},
	})
}

func (a *App) SaveWorkbook(request SaveWorkbookRequest) (string, error) {
	inputPath, err := existingWorkbookPath(request.InputPath)
	if err != nil {
		return "", err
	}
	fromText, toText := strings.TrimSpace(request.From), strings.TrimSpace(request.To)
	if fromText == "" {
		fromText = strings.TrimSpace(request.Date)
	}
	if fromText == "" {
		fromText = time.Now().Format("2006-01-02")
	}
	if toText == "" {
		toText = fromText
	}
	from, to, err := parseRequiredRange(fromText, toText)
	if err != nil {
		return "", err
	}
	dateSuffix := from.Format("20060102")
	if !to.Equal(from) {
		dateSuffix += "-" + to.Format("20060102")
	}
	name := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath)) + "_filled_" + dateSuffix + ".xlsx"
	outputPath, err := a.dialogs.SaveFile(a.appContext(), fileDialogOptions{
		Title:            "Save filled workbook / 儲存已填寫活頁簿",
		DefaultDirectory: filepath.Dir(inputPath),
		DefaultFilename:  name,
		Filters:          []fileDialogFilter{{DisplayName: "Excel workbook (*.xlsx)", Pattern: "*.xlsx"}},
	})
	if err != nil || outputPath == "" {
		return outputPath, err
	}
	if err := validateOutputPath(inputPath, outputPath); err != nil {
		return "", err
	}
	return outputPath, nil
}

func (a *App) ScanWorkbook(request ScanWorkbookRequest) (WorkbookScan, error) {
	inputPath, err := existingWorkbookPath(request.InputPath)
	if err != nil {
		return WorkbookScan{}, err
	}
	if _, err := loadMapper(request.MappingPath); err != nil {
		return WorkbookScan{}, err
	}
	fromText, toText := request.From, request.To
	if strings.TrimSpace(fromText) == "" {
		fromText = request.Date
	}
	from, to, err := parseOptionalRange(fromText, toText)
	if err != nil {
		return WorkbookScan{}, err
	}
	operationID, err := newUUID()
	if err != nil {
		return WorkbookScan{}, err
	}
	a.emit(operationID, "scan", 0, 1, "Scanning workbook / 正在掃描活頁簿")
	sheet := strings.TrimSpace(request.Sheet)
	if sheet == "" {
		sheet = strings.TrimSpace(request.SheetName)
	}
	scan, err := a.engine.Scan(engineScanRequest{InputPath: inputPath, Sheet: sheet, From: from, To: to})
	if err != nil {
		return WorkbookScan{}, err
	}
	profiles, err := a.ListProfiles()
	if err != nil {
		return WorkbookScan{}, err
	}
	for _, profile := range profiles {
		if profile.Enabled && profile.HasCredentials {
			scan.AvailableProfiles++
		}
	}
	scan.InputPath = inputPath
	scan.FileName = filepath.Base(inputPath)
	if scan.SheetName == "" {
		scan.SheetName = sheet
	}
	scan.Rows, scan.Stores, scan.Jobs = scan.RowCount, scan.StoreCount, scan.JobCount
	scan.Accounts = scan.AvailableProfiles
	a.emit(operationID, "scan", 1, 1, "Workbook scan complete / 活頁簿掃描完成")
	return scan, nil
}

func (a *App) ListProfiles() ([]Profile, error) {
	a.profileMu.Lock()
	defer a.profileMu.Unlock()
	return a.listProfilesLocked()
}

func (a *App) listProfilesLocked() ([]Profile, error) {
	records, err := a.profiles.List()
	if err != nil {
		return nil, err
	}
	result := make([]Profile, 0, len(records))
	for _, record := range records {
		_, credentialErr := a.credentials.Get(record.ID)
		hasCredentials := credentialErr == nil
		if credentialErr != nil && !errors.Is(credentialErr, securestore.ErrNotFound) {
			return nil, credentialErr
		}
		result = append(result, profileFromRecord(record, hasCredentials))
	}
	return result, nil
}

func (a *App) CreateOrUpdateProfile(request ProfileUpsertRequest) (Profile, error) {
	finishMutation, err := a.beginProfileMutation()
	if err != nil {
		return Profile{}, err
	}
	defer finishMutation()
	displayName := strings.TrimSpace(request.DisplayName)
	if displayName == "" || len([]rune(displayName)) > 80 {
		return Profile{}, errors.New("displayName is required and must not exceed 80 characters")
	}
	a.profileMu.Lock()
	defer a.profileMu.Unlock()
	records, err := a.profiles.List()
	if err != nil {
		return Profile{}, err
	}
	index := -1
	for candidate := range records {
		if records[candidate].ID == request.ID {
			index = candidate
			break
		}
	}
	isNew := strings.TrimSpace(request.ID) == ""
	if isNew {
		request.ID, err = newUUID()
		if err != nil {
			return Profile{}, err
		}
		index = len(records)
		records = append(records, profileRecord{ID: request.ID, Priority: len(records)})
	} else if index < 0 || !validProfileID(request.ID) {
		return Profile{}, errors.New("profile does not exist")
	}
	accountInput := strings.TrimSpace(request.Account)
	passwordInput := request.Password
	credentialUpdateRequested := accountInput != "" || passwordInput != ""
	if isNew && (accountInput == "" || passwordInput == "") {
		return Profile{}, errors.New("account and password are required for a new profile")
	}
	var previous securestore.Credential
	previousExists := false
	if existing, getErr := a.credentials.Get(request.ID); getErr == nil {
		previous, previousExists = existing, true
	} else if !errors.Is(getErr, securestore.ErrNotFound) {
		return Profile{}, getErr
	}
	hasCredentials := credentialUpdateRequested || previousExists
	if request.Enabled && !hasCredentials {
		return Profile{}, errors.New("credentials are required before enabling a profile")
	}
	if credentialUpdateRequested {
		nextCredential := previous
		if accountInput != "" {
			nextCredential.Account = accountInput
		}
		if passwordInput != "" {
			nextCredential.Password = passwordInput
		}
		if strings.TrimSpace(nextCredential.Account) == "" || nextCredential.Password == "" {
			return Profile{}, errors.New("account and password are required when no saved credentials exist")
		}
		credentialChanged := !previousExists || previous.Account != nextCredential.Account || previous.Password != nextCredential.Password
		if credentialChanged {
			// A cookie session is authenticated independently of the supplied
			// credentials. Remove it first so a changed profile can never continue
			// silently as the previous account. Losing a stale session if a later
			// metadata write fails is safe; the next test/analyze simply signs in.
			if err := a.cookies.DeleteCookie(request.ID); err != nil {
				return Profile{}, err
			}
		}
		if err := a.credentials.Put(request.ID, nextCredential); err != nil {
			return Profile{}, err
		}
	}
	records[index].DisplayName = displayName
	records[index].Enabled = request.Enabled
	if err := a.profiles.Replace(records); err != nil {
		if credentialUpdateRequested {
			var rollbackErr error
			if previousExists {
				rollbackErr = a.credentials.Put(request.ID, previous)
			} else {
				rollbackErr = a.credentials.Delete(request.ID)
			}
			if rollbackErr != nil {
				return Profile{}, errors.Join(err, fmt.Errorf("credential rollback failed: %w", rollbackErr))
			}
		}
		return Profile{}, err
	}
	return profileFromRecord(records[index], hasCredentials), nil
}

func (a *App) TestProfile(request TestProfileRequest) (ProfileTestResult, error) {
	if !validProfileID(request.ProfileID) {
		return ProfileTestResult{}, errors.New("invalid profile identifier")
	}
	operationID, err := newUUID()
	if err != nil {
		return ProfileTestResult{}, err
	}
	ctx, cancel := context.WithTimeout(a.appContext(), 3*time.Minute)
	a.operationMu.Lock()
	if (a.active != nil && a.active.running) || a.profileMutationRunning || a.profileTestRunning || a.salesAnalysisRunning {
		a.operationMu.Unlock()
		cancel()
		return ProfileTestResult{}, errors.New("another account or workbook operation is already running")
	}
	a.profileTestRunning = true
	a.profileTestID = operationID
	a.profileTestCancel = cancel
	a.operationMu.Unlock()
	a.profileMu.Lock()
	defer func() {
		a.profileMu.Unlock()
		cancel()
		a.operationMu.Lock()
		if a.profileTestID == operationID {
			a.profileTestRunning = false
			a.profileTestID = ""
			a.profileTestCancel = nil
		}
		a.operationMu.Unlock()
	}()
	records, err := a.profiles.List()
	if err != nil {
		return ProfileTestResult{}, err
	}
	if _, ok := findProfile(records, request.ProfileID); !ok {
		return ProfileTestResult{}, errors.New("profile does not exist")
	}
	credential, err := a.credentials.Get(request.ProfileID)
	if err != nil {
		return ProfileTestResult{}, err
	}
	cookies, err := a.cookies.CookieStore(request.ProfileID)
	if err != nil {
		return ProfileTestResult{}, err
	}
	client, err := a.clients.New(credential, cookies)
	if err != nil {
		return ProfileTestResult{}, err
	}
	a.emit(operationID, "login", 0, 1, "Testing sign-in / 正在測試登入")
	stores, err := client.Stores(ctx)
	if err != nil {
		return ProfileTestResult{}, err
	}
	a.emit(operationID, "stores", 1, 1, "Authorized stores loaded / 已載入授權門店")
	return ProfileTestResult{
		ProfileID: request.ProfileID, StoreCount: len(stores), OK: true, Success: true,
		Message: "Sign-in succeeded / 登入成功",
	}, nil
}

func (a *App) DeleteProfile(request ProfileIDRequest) error {
	finishMutation, err := a.beginProfileMutation()
	if err != nil {
		return err
	}
	defer finishMutation()
	a.profileMu.Lock()
	defer a.profileMu.Unlock()
	records, err := a.profiles.List()
	if err != nil {
		return err
	}
	index, ok := findProfile(records, request.ProfileID)
	if !ok {
		return errors.New("profile does not exist")
	}
	credential, credentialErr := a.credentials.Get(request.ProfileID)
	hadCredential := credentialErr == nil
	if credentialErr != nil && !errors.Is(credentialErr, securestore.ErrNotFound) {
		return credentialErr
	}
	cookieStore, err := a.cookies.CookieStore(request.ProfileID)
	if err != nil {
		return err
	}
	cookieData, err := cookieStore.Load()
	if err != nil {
		return err
	}
	defer func() {
		for index := range cookieData {
			cookieData[index] = 0
		}
	}()
	restoreSecrets := func() error {
		var rollbackErrors []error
		if hadCredential {
			if restoreErr := a.credentials.Put(request.ProfileID, credential); restoreErr != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("restore credential: %w", restoreErr))
			}
		}
		if len(cookieData) > 0 {
			restoredStore, restoreErr := a.cookies.CookieStore(request.ProfileID)
			if restoreErr == nil {
				restoreErr = restoredStore.Save(cookieData)
			}
			if restoreErr != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("restore cookie session: %w", restoreErr))
			}
		}
		return errors.Join(rollbackErrors...)
	}
	if err := a.credentials.Delete(request.ProfileID); err != nil {
		return err
	}
	if err := a.cookies.DeleteCookie(request.ProfileID); err != nil {
		if rollbackErr := restoreSecrets(); rollbackErr != nil {
			return errors.Join(err, fmt.Errorf("profile deletion rollback failed: %w", rollbackErr))
		}
		return err
	}
	records = append(records[:index], records[index+1:]...)
	if err := a.profiles.Replace(records); err != nil {
		if rollbackErr := restoreSecrets(); rollbackErr != nil {
			return errors.Join(err, fmt.Errorf("profile deletion rollback failed: %w", rollbackErr))
		}
		return err
	}
	return nil
}

func (a *App) Reorder(request ReorderProfilesRequest) ([]Profile, error) {
	finishMutation, err := a.beginProfileMutation()
	if err != nil {
		return nil, err
	}
	defer finishMutation()
	a.profileMu.Lock()
	defer a.profileMu.Unlock()
	records, err := a.profiles.List()
	if err != nil {
		return nil, err
	}
	if len(request.ProfileIDs) != len(records) {
		return nil, errors.New("profileIds must contain every profile exactly once")
	}
	byID := make(map[string]profileRecord, len(records))
	for _, record := range records {
		byID[record.ID] = record
	}
	ordered := make([]profileRecord, 0, len(records))
	for priority, id := range request.ProfileIDs {
		record, ok := byID[id]
		if !ok {
			return nil, errors.New("profileIds contains an unknown or duplicate profile")
		}
		delete(byID, id)
		record.Priority = priority
		ordered = append(ordered, record)
	}
	if err := a.profiles.Replace(ordered); err != nil {
		return nil, err
	}
	return a.profilesWithCredentials(ordered)
}

func (a *App) Enable(request EnableProfileRequest) (Profile, error) {
	finishMutation, err := a.beginProfileMutation()
	if err != nil {
		return Profile{}, err
	}
	defer finishMutation()
	a.profileMu.Lock()
	defer a.profileMu.Unlock()
	records, err := a.profiles.List()
	if err != nil {
		return Profile{}, err
	}
	index, ok := findProfile(records, request.ProfileID)
	if !ok {
		return Profile{}, errors.New("profile does not exist")
	}
	_, credentialErr := a.credentials.Get(request.ProfileID)
	hasCredentials := credentialErr == nil
	if credentialErr != nil && !errors.Is(credentialErr, securestore.ErrNotFound) {
		return Profile{}, credentialErr
	}
	if request.Enabled && !hasCredentials {
		return Profile{}, errors.New("credentials are required before enabling a profile")
	}
	records[index].Enabled = request.Enabled
	if err := a.profiles.Replace(records); err != nil {
		return Profile{}, err
	}
	return profileFromRecord(records[index], hasCredentials), nil
}

func (a *App) Analyze(request AnalyzeRequest) (AnalysisResult, error) {
	inputPath, err := existingWorkbookPath(request.InputPath)
	if err != nil {
		return AnalysisResult{}, err
	}
	from, to, err := parseAnalyzeRange(request)
	if err != nil {
		return AnalysisResult{}, err
	}
	mapper, err := loadMapper(request.MappingPath)
	if err != nil {
		return AnalysisResult{}, err
	}
	maxJobs := request.MaxJobs
	if maxJobs == 0 {
		maxJobs = request.MaxQueries
	}
	if maxJobs < 0 {
		return AnalysisResult{}, errors.New("maxJobs must not be negative")
	}
	concurrency := request.AccountConcurrency
	if concurrency == 0 {
		concurrency = xlsxfill.DefaultConcurrency
	}
	if concurrency < 1 || concurrency > xlsxfill.MaximumConcurrency {
		return AnalysisResult{}, fmt.Errorf("query concurrency must be between 1 and %d", xlsxfill.MaximumConcurrency)
	}
	operationID, err := newUUID()
	if err != nil {
		return AnalysisResult{}, err
	}
	ctx, cancel := context.WithCancel(a.appContext())
	state := &operationState{id: operationID, inputPath: inputPath, allowPartial: request.AllowPartial, running: true, cancel: cancel}
	a.operationMu.Lock()
	if (a.active != nil && a.active.running) || a.profileMutationRunning || a.profileTestRunning || a.salesAnalysisRunning {
		a.operationMu.Unlock()
		cancel()
		return AnalysisResult{}, errors.New("another account or workbook operation is already running")
	}
	a.active = state
	a.operationMu.Unlock()

	fail := func(workErr error) (AnalysisResult, error) {
		cancel()
		a.operationMu.Lock()
		if a.active == state {
			a.active = nil
		}
		a.operationMu.Unlock()
		return AnalysisResult{}, workErr
	}
	a.emit(operationID, "scan", 0, 1, "Preparing workbook plan / 正在準備活頁簿計畫")
	router, allowedStores, ownership, overlapCount, warnings, err := a.buildRouter(ctx, operationID)
	if err != nil {
		return fail(err)
	}
	state.overlapCount = overlapCount
	state.warnings = warnings
	state.ownership = ownership
	sheet := strings.TrimSpace(request.Sheet)
	if sheet == "" {
		sheet = strings.TrimSpace(request.SheetName)
	}
	plan, err := a.engine.Analyze(ctx, router, engineAnalyzeRequest{
		InputPath: inputPath, Sheet: sheet, From: from, To: to,
		Overwrite: request.Overwrite, MaxJobs: maxJobs, Concurrency: concurrency,
		AllowedBusinessStoreIDs: allowedStores, Mapper: mapper,
		Progress: func(progress engineProgress) {
			a.emitQueryProgress(operationID, progress, "Querying sales data / 正在查詢銷售資料")
		},
	})
	if err != nil {
		return fail(err)
	}
	cancel()
	decoratePreview(plan.Preview, mapper, ownership)
	a.operationMu.Lock()
	if a.active != state {
		a.operationMu.Unlock()
		return AnalysisResult{}, errors.New("workbook operation was replaced")
	}
	state.plan = plan
	state.running = false
	state.cancel = nil
	a.operationMu.Unlock()
	a.emit(operationID, "preview", len(plan.Preview), len(plan.Preview), "Preview ready / 預覽已就緒")
	return analysisResult(state), nil
}

func (a *App) Cancel(request OperationRequest) error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	if a.active != nil && a.active.id == request.OperationID {
		if !a.active.running || a.active.cancel == nil {
			return errors.New("operation is not running")
		}
		a.active.cancel()
		return nil
	}
	if a.profileTestRunning && a.profileTestID == request.OperationID && a.profileTestCancel != nil {
		a.profileTestCancel()
		return nil
	}
	return errors.New("operation does not exist")
}

func (a *App) RetryFailed(request OperationRequest) (AnalysisResult, error) {
	state, ctx, finish, err := a.beginExistingWork(request.OperationID)
	if err != nil {
		return AnalysisResult{}, err
	}
	if state.plan.RetryableCount == 0 {
		finish(nil, errors.New("plan has no failed work to retry"))
		return AnalysisResult{}, errors.New("plan has no failed work to retry")
	}
	plan, retryErr := a.engine.RetryFailed(ctx, state.plan, func(progress engineProgress) {
		a.emitQueryProgress(state.id, progress, "Retrying failed queries / 正在重試失敗查詢")
	})
	if retryErr != nil {
		finish(nil, retryErr)
		return AnalysisResult{}, retryErr
	}
	decoratePreview(plan.Preview, xlsxfill.IdentityStoreMap{}, state.ownership)
	a.emit(state.id, "preview", len(plan.Preview), len(plan.Preview), "Retry preview ready / 重試預覽已就緒")
	finish(plan, nil)
	return analysisResult(state), nil
}

func (a *App) Apply(request ApplyRequest) (ApplyResult, error) {
	state, ctx, finish, err := a.beginExistingWork(request.OperationID)
	if err != nil {
		return ApplyResult{}, err
	}
	outputPath := strings.TrimSpace(request.OutputPath)
	if err := validateOutputPath(state.inputPath, outputPath); err != nil {
		finish(nil, err)
		return ApplyResult{}, err
	}
	allowPartial := request.AllowPartial || state.allowPartial
	if allowPartial && !request.KeepIssueOriginal {
		err := errors.New("keepIssueOriginal must be true when allowPartial is enabled")
		finish(nil, err)
		return ApplyResult{}, err
	}
	if !state.plan.Complete {
		err := errors.New("plan is incomplete; retry pending work before applying")
		finish(nil, err)
		return ApplyResult{}, err
	}
	if state.plan.AggregateProblemCount > 0 {
		err := errors.New("plan has aggregate problems that cannot be partially applied")
		finish(nil, err)
		return ApplyResult{}, err
	}
	if state.plan.ProblemCount > 0 && !allowPartial {
		err := errors.New("plan has unresolved problems; enable allowPartial or retry first")
		finish(nil, err)
		return ApplyResult{}, err
	}
	result, applyErr := a.engine.Apply(ctx, state.plan, outputPath, allowPartial)
	if applyErr == nil {
		a.operationMu.Lock()
		if a.active == state {
			state.applied = true
		}
		a.operationMu.Unlock()
	}
	finish(state.plan, applyErr)
	if applyErr != nil {
		return ApplyResult{}, applyErr
	}
	return ApplyResult{
		OperationID: state.id, OutputPath: outputPath, Complete: result.Complete,
		ChangedCellCount: result.ChangedCellCount, ProblemCount: result.ProblemCount,
		WroteWorkbook: result.WroteWorkbook, ChangedCells: result.ChangedCellCount,
		SkippedRows: result.ProblemCount,
	}, nil
}

func (a *App) beginExistingWork(operationID string) (*operationState, context.Context, func(*enginePlan, error), error) {
	a.operationMu.Lock()
	if a.profileMutationRunning || a.profileTestRunning || a.salesAnalysisRunning {
		a.operationMu.Unlock()
		return nil, nil, nil, errors.New("another account operation is already running")
	}
	if a.active == nil || a.active.id != operationID || a.active.plan == nil {
		a.operationMu.Unlock()
		return nil, nil, nil, errors.New("operation plan does not exist")
	}
	state := a.active
	if state.running {
		a.operationMu.Unlock()
		return nil, nil, nil, errors.New("operation is already running")
	}
	if state.applied {
		a.operationMu.Unlock()
		return nil, nil, nil, errors.New("operation was already applied")
	}
	ctx, cancel := context.WithCancel(a.appContext())
	state.running = true
	state.cancel = cancel
	a.operationMu.Unlock()
	finish := func(plan *enginePlan, workErr error) {
		cancel()
		a.operationMu.Lock()
		defer a.operationMu.Unlock()
		if a.active != state {
			return
		}
		state.running = false
		state.cancel = nil
		if workErr == nil && plan != nil {
			state.plan = plan
		}
	}
	return state, ctx, finish, nil
}

func (a *App) buildRouter(ctx context.Context, operationID string) (*xlsxfill.ProviderRouter, []string, map[string]string, int, []string, error) {
	a.profileMu.Lock()
	records, err := a.profiles.List()
	a.profileMu.Unlock()
	if err != nil {
		return nil, nil, nil, 0, nil, err
	}
	enabled := make([]profileRecord, 0, len(records))
	for _, record := range records {
		if record.Enabled {
			enabled = append(enabled, record)
		}
	}
	if len(enabled) == 0 {
		return nil, nil, nil, 0, nil, errors.New("at least one enabled profile is required")
	}
	routes := make(map[string]xlsxfill.ProviderRoute)
	ownership := make(map[string]string)
	overlapped := make(map[string]struct{})
	for index, profile := range enabled {
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, 0, nil, err
		}
		a.emit(operationID, "login", index, len(enabled), "Signing in to account / 正在登入帳號")
		credential, err := a.credentials.Get(profile.ID)
		if errors.Is(err, securestore.ErrNotFound) {
			return nil, nil, nil, 0, nil, fmt.Errorf("enabled profile %q has no saved credentials", profile.DisplayName)
		}
		if err != nil {
			return nil, nil, nil, 0, nil, err
		}
		cookieStore, err := a.cookies.CookieStore(profile.ID)
		if err != nil {
			return nil, nil, nil, 0, nil, err
		}
		client, err := a.clients.New(credential, cookieStore)
		if err != nil {
			return nil, nil, nil, 0, nil, err
		}
		stores, err := client.Stores(ctx)
		if err != nil {
			return nil, nil, nil, 0, nil, err
		}
		a.emit(operationID, "login", index+1, len(enabled), "Account ready / 帳號已就緒")
		for _, store := range stores {
			storeID := strings.TrimSpace(store.BusinessID)
			if storeID == "" {
				continue
			}
			if _, exists := routes[storeID]; exists {
				overlapped[storeID] = struct{}{}
				continue
			}
			routes[storeID] = xlsxfill.ProviderRoute{
				Provider: client, Profile: profile.DisplayName,
			}
			ownership[storeID] = profile.DisplayName
		}
	}
	a.emit(operationID, "stores", 1, 1, "Authorized stores ready / 授權門店已就緒")
	router, err := xlsxfill.NewProfiledProviderRouter(routes)
	if err != nil {
		return nil, nil, nil, 0, nil, err
	}
	allowed := make([]string, 0, len(routes))
	for storeID := range routes {
		allowed = append(allowed, storeID)
	}
	sort.Strings(allowed)
	warnings := []string{}
	if len(overlapped) > 0 {
		warnings = append(warnings, fmt.Sprintf("%d authorized store overlap(s) use profile priority / %d 個授權門店重疊，已依帳號優先順序選用", len(overlapped), len(overlapped)))
	}
	return router, allowed, ownership, len(overlapped), warnings, nil
}

func (a *App) beginProfileMutation() (func(), error) {
	a.operationMu.Lock()
	if (a.active != nil && a.active.running) || a.profileMutationRunning || a.profileTestRunning || a.salesAnalysisRunning {
		a.operationMu.Unlock()
		return nil, errors.New("profiles cannot be changed while an account or workbook operation is running")
	}
	a.active = nil
	a.profileMutationRunning = true
	a.operationMu.Unlock()
	return func() {
		a.operationMu.Lock()
		a.profileMutationRunning = false
		a.operationMu.Unlock()
	}, nil
}

func (a *App) rejectWhileRunning() error {
	a.operationMu.Lock()
	defer a.operationMu.Unlock()
	if (a.active != nil && a.active.running) || a.profileMutationRunning || a.profileTestRunning || a.salesAnalysisRunning {
		return errors.New("another account or workbook operation is already running")
	}
	return nil
}

func (a *App) profilesWithCredentials(records []profileRecord) ([]Profile, error) {
	result := make([]Profile, 0, len(records))
	for _, record := range records {
		_, err := a.credentials.Get(record.ID)
		if err != nil && !errors.Is(err, securestore.ErrNotFound) {
			return nil, err
		}
		result = append(result, profileFromRecord(record, err == nil))
	}
	return result, nil
}

func profileFromRecord(record profileRecord, hasCredentials bool) Profile {
	return Profile{ID: record.ID, DisplayName: record.DisplayName, Enabled: record.Enabled, Priority: record.Priority, HasCredentials: hasCredentials}
}

func findProfile(records []profileRecord, id string) (int, bool) {
	for index := range records {
		if records[index].ID == id {
			return index, true
		}
	}
	return -1, false
}

func loadMapper(path string) (xlsxfill.StoreMapper, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return xlsxfill.IdentityStoreMap{}, nil
	}
	extension := strings.ToLower(filepath.Ext(path))
	if extension != ".json" && extension != ".csv" {
		return nil, errors.New("mappingPath must be a .json or .csv file")
	}
	mapper, err := xlsxfill.LoadStoreMap(path)
	if err != nil {
		return nil, err
	}
	return mapper, nil
}

func parseAnalyzeRange(request AnalyzeRequest) (time.Time, time.Time, error) {
	fromText, toText := strings.TrimSpace(request.From), strings.TrimSpace(request.To)
	if fromText == "" {
		fromText = strings.TrimSpace(request.Date)
	}
	if toText == "" {
		toText = fromText
	}
	if fromText == "" {
		return time.Time{}, time.Time{}, errors.New("from is required")
	}
	return parseRequiredRange(fromText, toText)
}

func parseOptionalRange(fromText, toText string) (time.Time, time.Time, error) {
	fromText, toText = strings.TrimSpace(fromText), strings.TrimSpace(toText)
	if fromText == "" && toText == "" {
		return time.Time{}, time.Time{}, nil
	}
	if fromText == "" {
		fromText = toText
	}
	if toText == "" {
		toText = fromText
	}
	return parseRequiredRange(fromText, toText)
}

func parseRequiredRange(fromText, toText string) (time.Time, time.Time, error) {
	from, err := time.ParseInLocation("2006-01-02", fromText, time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("from must use YYYY-MM-DD")
	}
	to, err := time.ParseInLocation("2006-01-02", toText, time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("to must use YYYY-MM-DD")
	}
	if to.Before(from) {
		return time.Time{}, time.Time{}, errors.New("to must not precede from")
	}
	return from, to, nil
}

func existingWorkbookPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("inputPath is required")
	}
	if !strings.EqualFold(filepath.Ext(path), ".xlsx") {
		return "", errors.New("inputPath must be an .xlsx workbook")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve input workbook: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("inspect input workbook: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("inputPath must be a regular file")
	}
	return absolute, nil
}

func existingDirectoryPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("directory is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve directory: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("inspect directory: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("path must be a directory")
	}
	return absolute, nil
}

func validateOutputPath(inputPath, outputPath string) error {
	outputPath = strings.TrimSpace(outputPath)
	if outputPath == "" {
		return errors.New("outputPath is required")
	}
	if !strings.EqualFold(filepath.Ext(outputPath), ".xlsx") {
		return errors.New("outputPath must use the .xlsx extension")
	}
	inputAbsolute, err := filepath.Abs(inputPath)
	if err != nil {
		return fmt.Errorf("resolve input workbook: %w", err)
	}
	outputAbsolute, err := filepath.Abs(outputPath)
	if err != nil {
		return fmt.Errorf("resolve output workbook: %w", err)
	}
	if strings.EqualFold(filepath.Clean(inputAbsolute), filepath.Clean(outputAbsolute)) {
		return errors.New("outputPath must differ from inputPath; in-place overwrite is disabled")
	}
	return nil
}

func decoratePreview(rows []PreviewRow, mapper xlsxfill.StoreMapper, ownership map[string]string) {
	for index := range rows {
		if rows[index].ID == "" {
			rows[index].ID = fmt.Sprintf("%s:%d", rows[index].Date, rows[index].Row)
		}
		if rows[index].StoreLabel == "" {
			rows[index].StoreLabel = rows[index].WorkbookStoreID
		}
		businessID, ok := mapper.ResolveStore(strings.TrimSpace(rows[index].WorkbookStoreID))
		if ok && rows[index].ProfileDisplayName == "" {
			rows[index].ProfileDisplayName = ownership[strings.TrimSpace(businessID)]
		}
		if rows[index].ProfileLabel == "" {
			rows[index].ProfileLabel = rows[index].ProfileDisplayName
		}
		if rows[index].Message == "" && len(rows[index].IssueCodes) > 0 {
			rows[index].Message = rows[index].IssueCodes[0]
		}
	}
}

func analysisResult(state *operationState) AnalysisResult {
	plan := state.plan
	preview := append([]PreviewRow(nil), plan.Preview...)
	result := AnalysisResult{
		OperationID: state.id, PlanID: plan.PlanID, Complete: plan.Complete,
		OverlapCount: state.overlapCount, ProblemCount: plan.ProblemCount,
		AggregateProblemCount: plan.AggregateProblemCount, RetryableCount: plan.RetryableCount,
		ChangedCellCount: plan.ChangedCellCount, Warnings: append([]string(nil), state.warnings...),
		Preview: preview, Rows: append([]PreviewRow(nil), preview...), TotalCount: len(preview),
		CanApply: plan.Complete && plan.AggregateProblemCount == 0 && plan.ChangedCellCount > 0 && (plan.ProblemCount == 0 || state.allowPartial),
	}
	if len(result.Warnings) > 0 {
		result.OverlapWarning = result.Warnings[0]
	}
	for _, row := range preview {
		switch row.Status {
		case "change":
			result.ChangeCount++
		case "unchanged":
			result.UnchangedCount++
		case "failed":
			result.FailedCount++
		default:
			result.IssueCount++
		}
		if row.Message != "" {
			result.Issues = append(result.Issues, AnalysisIssue{
				Row: row.Row, Message: row.Message, Retryable: row.Status == "failed",
			})
		}
	}
	return result
}

func (a *App) emit(operationID, stage string, current, total int, message string) {
	a.events.Emit(a.appContext(), progressEventName, ProgressEvent{OperationID: operationID, Stage: stage, Current: current, Total: total, Message: message})
}

func (a *App) emitQueryProgress(operationID string, progress engineProgress, message string) {
	a.events.Emit(a.appContext(), progressEventName, ProgressEvent{
		OperationID: operationID,
		Stage:       "query",
		Current:     progress.Completed,
		Total:       progress.Total,
		Message:     message,
		Date:        progress.Date,
		StoreID:     progress.StoreID,
		Profile:     progress.Profile,
		Attempt:     progress.Attempt,
		Status:      progress.Status,
	})
}

func newUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("create identifier: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}

var _ rtasales.CookieStore = (*securestore.MemoryCookieStore)(nil)
