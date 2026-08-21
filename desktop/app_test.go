package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	rtasales "github.com/Miku0139oao/rta-sales-client-go/rtasales"
	"github.com/Miku0139oao/rta-sales-client-go/securestore"
	"github.com/Miku0139oao/rta-sales-client-go/xlsxfill"
	"github.com/xuri/excelize/v2"
)

type fakeDialogs struct {
	mu        sync.Mutex
	open      string
	directory string
	save      string
	lastOpen  fileDialogOptions
	lastSave  fileDialogOptions
}

func (d *fakeDialogs) OpenFile(_ context.Context, options fileDialogOptions) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastOpen = options
	return d.open, nil
}

func (d *fakeDialogs) OpenDirectory(context.Context, fileDialogOptions) (string, error) {
	return d.directory, nil
}

func (d *fakeDialogs) SaveFile(_ context.Context, options fileDialogOptions) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastSave = options
	return d.save, nil
}

type recordedEvent struct {
	name    string
	payload any
}

type fakeEvents struct {
	mu     sync.Mutex
	events []recordedEvent
}

func (e *fakeEvents) Emit(_ context.Context, name string, payload any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, recordedEvent{name: name, payload: payload})
}

type fakeRuntime struct{}

func (fakeRuntime) Check() RuntimeStatus {
	return RuntimeStatus{Available: true, Version: "test", Message: "available"}
}

type fakeCookies struct {
	mu     sync.Mutex
	stores map[string]*securestore.MemoryCookieStore
}

func (s *fakeCookies) CookieStore(profileID string) (rtasales.CookieStore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stores == nil {
		s.stores = make(map[string]*securestore.MemoryCookieStore)
	}
	if s.stores[profileID] == nil {
		s.stores[profileID] = new(securestore.MemoryCookieStore)
	}
	return s.stores[profileID], nil
}

func (s *fakeCookies) DeleteCookie(profileID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.stores, profileID)
	return nil
}

type fakeAccountClient struct {
	stores []rtasales.Store
	value  float64
}

func (c *fakeAccountClient) Stores(context.Context) ([]rtasales.Store, error) {
	return append([]rtasales.Store(nil), c.stores...), nil
}

func (c *fakeAccountClient) Sales(context.Context, rtasales.SalesQuery) (*rtasales.SalesResult, error) {
	transactions := c.value
	return &rtasales.SalesResult{TotalAmount: c.value, TotalTransactionCount: &transactions}, nil
}

type blockingStoresClient struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (c *blockingStoresClient) Stores(ctx context.Context) ([]rtasales.Store, error) {
	c.once.Do(func() { close(c.started) })
	select {
	case <-c.release:
		return []rtasales.Store{{BusinessID: "store-one"}}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *blockingStoresClient) Sales(context.Context, rtasales.SalesQuery) (*rtasales.SalesResult, error) {
	return nil, errors.New("sales is not used by profile tests")
}

type fakeClients struct {
	byAccount map[string]accountClient
}

func (f fakeClients) New(credential securestore.Credential, _ rtasales.CookieStore) (accountClient, error) {
	client := f.byAccount[credential.Account]
	if client == nil {
		return nil, errors.New("test account is not configured")
	}
	return client, nil
}

type replaceFailRepository struct {
	delegate profileRepository
	failNext bool
	err      error
}

type blockingReplaceRepository struct {
	delegate  profileRepository
	blockNext bool
	started   chan struct{}
	release   chan struct{}
}

func (r *blockingReplaceRepository) List() ([]profileRecord, error) {
	return r.delegate.List()
}

func (r *blockingReplaceRepository) Replace(records []profileRecord) error {
	if r.blockNext {
		r.blockNext = false
		close(r.started)
		<-r.release
	}
	return r.delegate.Replace(records)
}

func (r *replaceFailRepository) List() ([]profileRecord, error) {
	return r.delegate.List()
}

func (r *replaceFailRepository) Replace(records []profileRecord) error {
	if r.failNext {
		r.failNext = false
		return r.err
	}
	return r.delegate.Replace(records)
}

type failNthPutCredentialStore struct {
	delegate  securestore.CredentialStore
	mu        sync.Mutex
	putCalls  int
	failPutAt int
	err       error
}

func (s *failNthPutCredentialStore) Get(profileID string) (securestore.Credential, error) {
	return s.delegate.Get(profileID)
}

func (s *failNthPutCredentialStore) Put(profileID string, credential securestore.Credential) error {
	s.mu.Lock()
	s.putCalls++
	call := s.putCalls
	s.mu.Unlock()
	if call == s.failPutAt {
		return s.err
	}
	return s.delegate.Put(profileID, credential)
}

func (s *failNthPutCredentialStore) Delete(profileID string) error {
	return s.delegate.Delete(profileID)
}

type sequenceClients struct {
	mu      sync.Mutex
	clients []accountClient
	next    int
}

func (f *sequenceClients) New(securestore.Credential, rtasales.CookieStore) (accountClient, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.next >= len(f.clients) {
		return nil, errors.New("test client sequence exhausted")
	}
	client := f.clients[f.next]
	f.next++
	return client, nil
}

type fakeEngine struct {
	analyze func(context.Context, xlsxfill.SalesProvider, engineAnalyzeRequest) (*enginePlan, error)
	retry   func(context.Context, *enginePlan, func(engineProgress)) (*enginePlan, error)
	apply   func(context.Context, *enginePlan, string, bool) (engineApplyResult, error)
}

func (f *fakeEngine) Scan(engineScanRequest) (WorkbookScan, error) {
	return WorkbookScan{Sheets: []SheetSummary{{Name: "Dairly"}}, DateMin: "2026-08-01", DateMax: "2026-08-02", RowCount: 2, StoreCount: 1, JobCount: 2}, nil
}

func (f *fakeEngine) Analyze(ctx context.Context, provider xlsxfill.SalesProvider, request engineAnalyzeRequest) (*enginePlan, error) {
	if f.analyze != nil {
		return f.analyze(ctx, provider, request)
	}
	return &enginePlan{PlanID: "test-plan", InputPath: request.InputPath, Complete: true}, nil
}

func (f *fakeEngine) RetryFailed(ctx context.Context, plan *enginePlan, progress func(engineProgress)) (*enginePlan, error) {
	if f.retry != nil {
		return f.retry(ctx, plan, progress)
	}
	return plan, nil
}

func (f *fakeEngine) Apply(ctx context.Context, plan *enginePlan, output string, allowPartial bool) (engineApplyResult, error) {
	if f.apply != nil {
		return f.apply(ctx, plan, output, allowPartial)
	}
	return engineApplyResult{Complete: plan.Complete, ChangedCellCount: plan.ChangedCellCount, ProblemCount: plan.ProblemCount, WroteWorkbook: true}, nil
}

func newTestApp(t *testing.T, engine batchEngine, clients clientFactory) (*App, string, *fakeEvents) {
	t.Helper()
	root := t.TempDir()
	repository, err := NewFileProfileRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	mancodes, err := NewFileManCodeRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	events := new(fakeEvents)
	app, err := newApp(appDependencies{
		profiles: repository, mancodes: mancodes, credentials: securestore.NewMemoryCredentialStore(),
		cookies: new(fakeCookies), clients: clients, engine: engine,
		dialogs: new(fakeDialogs), events: events, runtime: fakeRuntime{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return app, root, events
}

func testWorkbook(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input.xlsx")
	if err := os.WriteFile(path, []byte("test workbook placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestProfileMetadataExcludesCredentials(t *testing.T) {
	app, root, _ := newTestApp(t, new(fakeEngine), fakeClients{byAccount: map[string]accountClient{
		"account-one": &fakeAccountClient{stores: []rtasales.Store{{BusinessID: "store-one"}}},
	}})
	created, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		DisplayName: "Primary", Account: "account-one", Password: "password-one", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !validProfileID(created.ID) || !created.HasCredentials {
		t.Fatalf("unexpected created profile: %#v", created)
	}
	data, err := os.ReadFile(filepath.Join(root, "profiles.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "account-one") || strings.Contains(string(data), "password-one") {
		t.Fatal("profile metadata exposed credentials")
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if document["version"] != float64(profileFileVersion) {
		t.Fatalf("unexpected profile document: %s", data)
	}
	cookieStore, err := app.cookies.CookieStore(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := cookieStore.Save([]byte("existing authenticated session")); err != nil {
		t.Fatal(err)
	}
	updated, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		ID: created.ID, DisplayName: "Renamed", Enabled: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.DisplayName != "Renamed" || !updated.HasCredentials {
		t.Fatalf("unexpected updated profile: %#v", updated)
	}
	credential, err := app.credentials.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if credential.Account != "account-one" || credential.Password != "password-one" {
		t.Fatal("blank edit fields replaced the saved credential")
	}
	preservedCookieStore, err := app.cookies.CookieStore(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	preservedCookie, err := preservedCookieStore.Load()
	if err != nil || string(preservedCookie) != "existing authenticated session" {
		t.Fatalf("metadata-only edit changed the saved session: %q, %v", preservedCookie, err)
	}
	accountUpdated, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		ID: created.ID, DisplayName: "Second account", Account: "account-two", Enabled: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	credential, err = app.credentials.Get(created.ID)
	if err != nil || credential.Account != "account-two" || credential.Password != "password-one" {
		t.Fatalf("account-only update did not preserve the password: %#v, %v", credential, err)
	}
	if accountUpdated.DisplayName != "Second account" || !accountUpdated.HasCredentials {
		t.Fatalf("unexpected account-only update result: %#v", accountUpdated)
	}
	replacedCookieStore, err := app.cookies.CookieStore(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	replacedCookie, err := replacedCookieStore.Load()
	if err != nil || len(replacedCookie) != 0 {
		t.Fatalf("account-only update retained the previous authenticated session: bytes=%d err=%v", len(replacedCookie), err)
	}
	if err := replacedCookieStore.Save([]byte("authenticated after account update")); err != nil {
		t.Fatal(err)
	}
	passwordUpdated, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		ID: created.ID, DisplayName: "Second account", Password: "password-two", Enabled: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	credential, err = app.credentials.Get(created.ID)
	if err != nil || credential.Account != "account-two" || credential.Password != "password-two" {
		t.Fatalf("password-only update did not preserve the account: %#v, %v", credential, err)
	}
	if passwordUpdated.DisplayName != "Second account" || !passwordUpdated.HasCredentials {
		t.Fatalf("unexpected password-only update result: %#v", passwordUpdated)
	}
	passwordCookieStore, err := app.cookies.CookieStore(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	replacedCookie, err = passwordCookieStore.Load()
	if err != nil || len(replacedCookie) != 0 {
		t.Fatalf("password-only update retained the previous authenticated session: bytes=%d err=%v", len(replacedCookie), err)
	}
	if err := app.credentials.Delete(created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		ID: created.ID, DisplayName: "Missing credentials", Account: "account-three", Enabled: false,
	}); err == nil {
		t.Fatal("expected a partial credential update to fail when no saved credential exists")
	}
	if _, err := app.Enable(EnableProfileRequest{ProfileID: created.ID, Enabled: true}); err == nil {
		t.Fatal("expected enabling a profile without saved credentials to fail")
	}
	profiles, err := app.ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].Enabled || profiles[0].HasCredentials {
		t.Fatalf("failed enable changed profile metadata: %#v", profiles)
	}
	if _, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		ID: created.ID, DisplayName: "Must remain disabled", Enabled: true,
	}); err == nil {
		t.Fatal("expected metadata update to reject enabling a profile without saved credentials")
	}
	profiles, err = app.ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].DisplayName != "Second account" || profiles[0].Enabled || profiles[0].HasCredentials {
		t.Fatalf("rejected metadata update changed the profile: %#v", profiles)
	}
}

func TestProfileUpdateReportsMetadataAndCredentialRollbackFailures(t *testing.T) {
	metadataErr := errors.New("injected metadata failure")
	rollbackErr := errors.New("injected credential rollback failure")
	root := t.TempDir()
	fileRepository, err := NewFileProfileRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	mancodes, err := NewFileManCodeRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	repository := &replaceFailRepository{delegate: fileRepository, err: metadataErr}
	credentials := &failNthPutCredentialStore{
		delegate: securestore.NewMemoryCredentialStore(), failPutAt: 3, err: rollbackErr,
	}
	app, err := newApp(appDependencies{
		profiles: repository, mancodes: mancodes, credentials: credentials, cookies: new(fakeCookies),
		clients: fakeClients{byAccount: map[string]accountClient{}}, engine: new(fakeEngine),
		dialogs: new(fakeDialogs), events: new(fakeEvents), runtime: fakeRuntime{},
	})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		DisplayName: "Primary", Account: "account-one", Password: "password-one", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	repository.failNext = true
	_, err = app.CreateOrUpdateProfile(ProfileUpsertRequest{
		ID: profile.ID, DisplayName: "Updated", Account: "account-two", Password: "password-two", Enabled: true,
	})
	if !errors.Is(err, metadataErr) || !errors.Is(err, rollbackErr) {
		t.Fatalf("expected joined metadata and rollback errors, got %v", err)
	}
}

func TestProfileMutationBlocksAccountAndWorkbookOperations(t *testing.T) {
	root := t.TempDir()
	fileRepository, err := NewFileProfileRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	mancodes, err := NewFileManCodeRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	repository := &blockingReplaceRepository{
		delegate: fileRepository, started: make(chan struct{}), release: make(chan struct{}),
	}
	app, err := newApp(appDependencies{
		profiles: repository, mancodes: mancodes, credentials: securestore.NewMemoryCredentialStore(), cookies: new(fakeCookies),
		clients: fakeClients{byAccount: map[string]accountClient{
			"account-one": &fakeAccountClient{stores: []rtasales.Store{{BusinessID: "store-one"}}},
		}},
		engine: new(fakeEngine), dialogs: new(fakeDialogs), events: new(fakeEvents), runtime: fakeRuntime{},
	})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		DisplayName: "Primary", Account: "account-one", Password: "password-one", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	repository.blockNext = true
	done := make(chan error, 1)
	go func() {
		_, updateErr := app.CreateOrUpdateProfile(ProfileUpsertRequest{
			ID: profile.ID, DisplayName: "Renamed", Enabled: true,
		})
		done <- updateErr
	}()
	select {
	case <-repository.started:
	case <-time.After(time.Second):
		t.Fatal("profile mutation did not reach metadata replacement")
	}
	if _, err := app.Analyze(AnalyzeRequest{InputPath: testWorkbook(t), Date: "2026-08-01"}); err == nil {
		t.Fatal("expected analyze rejection while profile mutation is running")
	}
	if _, err := app.TestProfile(TestProfileRequest{ProfileID: profile.ID}); err == nil {
		t.Fatal("expected profile test rejection while profile mutation is running")
	}
	if _, err := app.Enable(EnableProfileRequest{ProfileID: profile.ID, Enabled: false}); err == nil {
		t.Fatal("expected concurrent profile mutation rejection")
	}
	close(repository.release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("profile mutation did not finish after release")
	}
}

func TestDeleteProfileRestoresSecretsWhenMetadataUpdateFails(t *testing.T) {
	metadataErr := errors.New("injected metadata failure")
	root := t.TempDir()
	fileRepository, err := NewFileProfileRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	mancodes, err := NewFileManCodeRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	repository := &replaceFailRepository{delegate: fileRepository, err: metadataErr}
	credentials := securestore.NewMemoryCredentialStore()
	cookies := new(fakeCookies)
	app, err := newApp(appDependencies{
		profiles: repository, mancodes: mancodes, credentials: credentials, cookies: cookies,
		clients: fakeClients{byAccount: map[string]accountClient{}}, engine: new(fakeEngine),
		dialogs: new(fakeDialogs), events: new(fakeEvents), runtime: fakeRuntime{},
	})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		DisplayName: "Primary", Account: "account-one", Password: "password-one", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	cookieStore, err := cookies.CookieStore(profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := cookieStore.Save([]byte("encrypted-session-fixture")); err != nil {
		t.Fatal(err)
	}
	repository.failNext = true
	if err := app.DeleteProfile(ProfileIDRequest{ProfileID: profile.ID}); !errors.Is(err, metadataErr) {
		t.Fatalf("expected metadata failure, got %v", err)
	}
	credential, err := credentials.Get(profile.ID)
	if err != nil || credential.Account != "account-one" || credential.Password != "password-one" {
		t.Fatalf("credential was not restored: %#v, %v", credential, err)
	}
	restoredStore, err := cookies.CookieStore(profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	restoredCookie, err := restoredStore.Load()
	if err != nil || string(restoredCookie) != "encrypted-session-fixture" {
		t.Fatalf("cookie was not restored: %q, %v", restoredCookie, err)
	}
	profiles, err := app.ListProfiles()
	if err != nil || len(profiles) != 1 || profiles[0].ID != profile.ID {
		t.Fatalf("profile metadata changed after failed deletion: %#v, %v", profiles, err)
	}
}

func TestExcelUsesFirstEnabledProfileAndReorderChangesOwner(t *testing.T) {
	engine := new(fakeEngine)
	engine.analyze = func(ctx context.Context, provider xlsxfill.SalesProvider, request engineAnalyzeRequest) (*enginePlan, error) {
		result, err := provider.Sales(ctx, rtasales.SalesQuery{BusinessStoreID: "shared-store"})
		if err != nil {
			return nil, err
		}
		value := formatNumber(&result.TotalAmount)
		return &enginePlan{
			PlanID: "test-plan", InputPath: request.InputPath, Complete: true, ChangedCellCount: 2,
			Preview: []PreviewRow{{Date: "2026-08-01", Row: 2, WorkbookStoreID: "shared-store", ProposedL: value, Status: "change"}},
		}, nil
	}
	clients := fakeClients{byAccount: map[string]accountClient{
		"account-one": &fakeAccountClient{stores: []rtasales.Store{{BusinessID: "shared-store"}}, value: 1},
		"account-two": &fakeAccountClient{stores: []rtasales.Store{{BusinessID: "shared-store"}}, value: 2},
	}}
	app, _, events := newTestApp(t, engine, clients)
	first, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{DisplayName: "First", Account: "account-one", Password: "password-one", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{DisplayName: "Second", Account: "account-two", Password: "password-two", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	input := testWorkbook(t)
	result, err := app.Analyze(AnalyzeRequest{InputPath: input, Date: "2026-08-01"})
	if err != nil {
		t.Fatal(err)
	}
	if result.OverlapCount != 0 || len(result.Preview) != 1 || result.Preview[0].ProposedL != "1" {
		t.Fatalf("first enabled profile should own every query: %#v", result)
	}
	if result.Preview[0].ProfileDisplayName != "First" {
		t.Fatalf("unexpected first owner: %#v", result.Preview[0])
	}
	ordered, err := app.Reorder(ReorderProfilesRequest{ProfileIDs: []string{second.ID, first.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if ordered[0].ID != second.ID || ordered[0].Priority != 0 || ordered[1].Priority != 1 {
		t.Fatalf("unexpected reordered profiles: %#v", ordered)
	}
	result, err = app.Analyze(AnalyzeRequest{InputPath: input, Date: "2026-08-01"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Preview[0].ProposedL != "2" || result.Preview[0].ProfileDisplayName != "Second" {
		t.Fatalf("reordered first profile was not used: %#v", result.Preview[0])
	}
	events.mu.Lock()
	defer events.mu.Unlock()
	for _, event := range events.events {
		encoded, err := json.Marshal(event.payload)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "account-one") || strings.Contains(string(encoded), "password-one") {
			t.Fatalf("progress event exposed credentials: %s", encoded)
		}
	}
}

func TestAnalyzeDoesNotQueryASecondEnabledProfile(t *testing.T) {
	second := &countingAccountClient{fakeAccountClient: fakeAccountClient{
		stores: []rtasales.Store{{BusinessID: "208", Label: "208"}}, value: 99,
	}}
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{byAccount: map[string]accountClient{
		"account-one": &fakeAccountClient{stores: []rtasales.Store{{BusinessID: "107"}, {BusinessID: "108"}}, value: 10},
		"account-two": second,
	}})
	if _, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{DisplayName: "Primary", Account: "account-one", Password: "password", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{DisplayName: "Spare", Account: "account-two", Password: "password", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	router, allowed, _, overlap, _, err := app.buildRouter(context.Background(), "one-account", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if overlap != 0 || len(allowed) != 2 || allowed[0] != "107" || allowed[1] != "108" {
		t.Fatalf("allowed=%v overlap=%d", allowed, overlap)
	}
	if _, ok := router.ProviderForStore("208"); ok {
		t.Fatal("second profile store should not be on the router")
	}
	if second.storesCalls > 0 || second.salesCalls > 0 {
		t.Fatalf("second profile was contacted: stores=%d sales=%d", second.storesCalls, second.salesCalls)
	}
}

type countingAccountClient struct {
	fakeAccountClient
	storesCalls int
	salesCalls  int
}

func (c *countingAccountClient) Stores(ctx context.Context) ([]rtasales.Store, error) {
	c.storesCalls++
	return c.fakeAccountClient.Stores(ctx)
}

func (c *countingAccountClient) Sales(ctx context.Context, query rtasales.SalesQuery) (*rtasales.SalesResult, error) {
	c.salesCalls++
	return c.fakeAccountClient.Sales(ctx, query)
}

func TestDuplicateAccountProfilesDoNotExposeAccountInRoutes(t *testing.T) {
	factory := &sequenceClients{clients: []accountClient{
		&fakeAccountClient{stores: []rtasales.Store{{BusinessID: "store-one"}}},
		&fakeAccountClient{stores: []rtasales.Store{{BusinessID: "store-two"}}},
	}}
	app, _, _ := newTestApp(t, new(fakeEngine), factory)
	for _, name := range []string{"First", "Second"} {
		if _, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
			DisplayName: name, Account: "same-account", Password: "password", Enabled: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	router, allowed, _, overlap, _, err := app.buildRouter(context.Background(), "test-operation", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if overlap != 0 || len(allowed) != 1 || allowed[0] != "store-one" {
		t.Fatalf("one account should only use the first profile stores: allowed=%v overlap=%d", allowed, overlap)
	}
	first, firstOK := router.ProviderForStore("store-one")
	_, secondOK := router.ProviderForStore("store-two")
	if !firstOK || secondOK {
		t.Fatalf("second profile should not be queried: firstOK=%v secondOK=%v", firstOK, secondOK)
	}
	if strings.Contains(first.Lane, "same-account") || strings.Contains(first.Profile, "same-account") {
		t.Fatal("route metadata exposed the account identifier")
	}
}

func TestAnalyzeEmitsUsefulQueryProgressWithoutCredentials(t *testing.T) {
	engine := new(fakeEngine)
	engine.analyze = func(_ context.Context, _ xlsxfill.SalesProvider, request engineAnalyzeRequest) (*enginePlan, error) {
		request.Progress(engineProgress{
			Completed: 7, Total: 12, Date: "2026-08-07", StoreID: "107",
			Profile: "Production", Attempt: 2, Status: "success",
		})
		return &enginePlan{PlanID: "test-plan", InputPath: request.InputPath, Complete: true}, nil
	}
	app, _, events := newTestApp(t, engine, fakeClients{byAccount: map[string]accountClient{
		"secret-account": &fakeAccountClient{stores: []rtasales.Store{{BusinessID: "107"}}},
	}})
	if _, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		DisplayName: "Production", Account: "secret-account", Password: "secret-password", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Analyze(AnalyzeRequest{InputPath: testWorkbook(t), Date: "2026-08-07"}); err != nil {
		t.Fatal(err)
	}
	events.mu.Lock()
	defer events.mu.Unlock()
	found := false
	for _, recorded := range events.events {
		event, ok := recorded.payload.(ProgressEvent)
		if !ok || event.Stage != "query" {
			continue
		}
		found = true
		if event.Current != 7 || event.Total != 12 || event.StoreID != "107" || event.Date != "2026-08-07" || event.Profile != "Production" || event.Attempt != 2 || event.Status != "success" {
			t.Fatalf("unexpected query progress: %#v", event)
		}
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "secret-account") || strings.Contains(string(encoded), "secret-password") {
			t.Fatalf("query progress exposed credentials: %s", encoded)
		}
	}
	if !found {
		t.Fatal("query progress event was not emitted")
	}
}

func TestMethodsRejectUnsafeOperationStates(t *testing.T) {
	started := make(chan struct{})
	engine := new(fakeEngine)
	engine.analyze = func(ctx context.Context, _ xlsxfill.SalesProvider, _ engineAnalyzeRequest) (*enginePlan, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	clients := fakeClients{byAccount: map[string]accountClient{
		"account-one": &fakeAccountClient{stores: []rtasales.Store{{BusinessID: "store-one"}}},
	}}
	app, _, _ := newTestApp(t, engine, clients)
	profile, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{DisplayName: "Primary", Account: "account-one", Password: "password-one", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	input := testWorkbook(t)
	done := make(chan error, 1)
	go func() {
		_, analyzeErr := app.Analyze(AnalyzeRequest{InputPath: input, Date: "2026-08-01"})
		done <- analyzeErr
	}()
	<-started
	if _, err := app.Enable(EnableProfileRequest{ProfileID: profile.ID, Enabled: false}); err == nil {
		t.Fatal("expected profile mutation rejection while analyze is running")
	}
	if _, err := app.Analyze(AnalyzeRequest{InputPath: input, Date: "2026-08-01"}); err == nil {
		t.Fatal("expected concurrent analyze rejection")
	}
	app.operationMu.Lock()
	operationID := app.active.id
	app.operationMu.Unlock()
	if err := app.Cancel(OperationRequest{OperationID: operationID}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled analyze, got %v", err)
	}
	if _, err := app.Apply(ApplyRequest{OperationID: operationID, OutputPath: filepath.Join(t.TempDir(), "output.xlsx")}); err == nil {
		t.Fatal("expected apply rejection after canceled analysis")
	}
}

func TestProfileTestBlocksMutationsAndWorkbookOperations(t *testing.T) {
	client := &blockingStoresClient{started: make(chan struct{}), release: make(chan struct{})}
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{byAccount: map[string]accountClient{
		"account-one": client,
	}})
	profile, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		DisplayName: "Primary", Account: "account-one", Password: "password-one", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	type testResponse struct {
		result ProfileTestResult
		err    error
	}
	done := make(chan testResponse, 1)
	go func() {
		result, testErr := app.TestProfile(TestProfileRequest{ProfileID: profile.ID})
		done <- testResponse{result: result, err: testErr}
	}()
	select {
	case <-client.started:
	case <-time.After(time.Second):
		t.Fatal("profile test did not reach the store request")
	}

	if _, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		ID: profile.ID, DisplayName: "Renamed", Enabled: true,
	}); err == nil {
		t.Fatal("expected profile edit rejection while profile test is running")
	}
	if err := app.DeleteProfile(ProfileIDRequest{ProfileID: profile.ID}); err == nil {
		t.Fatal("expected profile deletion rejection while profile test is running")
	}
	if _, err := app.Analyze(AnalyzeRequest{
		InputPath: testWorkbook(t), Date: "2026-08-01",
	}); err == nil {
		t.Fatal("expected workbook analysis rejection while profile test is running")
	}
	if _, err := app.TestProfile(TestProfileRequest{ProfileID: profile.ID}); err == nil {
		t.Fatal("expected concurrent profile test rejection")
	}

	close(client.release)
	select {
	case response := <-done:
		if response.err != nil {
			t.Fatal(response.err)
		}
		if !response.result.Success || response.result.StoreCount != 1 {
			t.Fatalf("unexpected profile test result: %#v", response.result)
		}
	case <-time.After(time.Second):
		t.Fatal("profile test did not finish after the client was released")
	}
	if _, err := app.Enable(EnableProfileRequest{ProfileID: profile.ID, Enabled: false}); err != nil {
		t.Fatalf("profile operation lock was not released: %v", err)
	}
}

func TestCancelStopsActiveProfileTest(t *testing.T) {
	client := &blockingStoresClient{started: make(chan struct{}), release: make(chan struct{})}
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{byAccount: map[string]accountClient{
		"account-one": client,
	}})
	profile, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		DisplayName: "Primary", Account: "account-one", Password: "password-one", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, testErr := app.TestProfile(TestProfileRequest{ProfileID: profile.ID})
		done <- testErr
	}()
	select {
	case <-client.started:
	case <-time.After(time.Second):
		t.Fatal("profile test did not reach the store request")
	}
	app.operationMu.Lock()
	operationID := app.profileTestID
	app.operationMu.Unlock()
	if operationID == "" {
		t.Fatal("profile test did not publish an operation identifier")
	}
	if err := app.Cancel(OperationRequest{OperationID: operationID}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected canceled profile test, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not stop the profile test")
	}
	if _, err := app.Enable(EnableProfileRequest{ProfileID: profile.ID, Enabled: false}); err != nil {
		t.Fatalf("profile operation lock was not released after cancel: %v", err)
	}
}

func TestApplyAndRetryRejectInvalidPlanUsage(t *testing.T) {
	clients := fakeClients{byAccount: map[string]accountClient{
		"account-one": &fakeAccountClient{stores: []rtasales.Store{{BusinessID: "store-one"}}},
	}}
	engine := new(fakeEngine)
	engine.analyze = func(_ context.Context, _ xlsxfill.SalesProvider, request engineAnalyzeRequest) (*enginePlan, error) {
		return &enginePlan{PlanID: "complete-plan", InputPath: request.InputPath, Complete: true, ChangedCellCount: 2}, nil
	}
	app, _, _ := newTestApp(t, engine, clients)
	if _, err := app.Apply(ApplyRequest{OperationID: "missing", OutputPath: "output.xlsx"}); err == nil {
		t.Fatal("expected missing plan rejection")
	}
	if _, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{DisplayName: "Primary", Account: "account-one", Password: "password-one", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	input := testWorkbook(t)
	analysis, err := app.Analyze(AnalyzeRequest{InputPath: input, Date: "2026-08-01"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.RetryFailed(OperationRequest{OperationID: analysis.OperationID}); err == nil {
		t.Fatal("expected retry rejection for complete plan")
	}
	if _, err := app.Apply(ApplyRequest{OperationID: analysis.OperationID, OutputPath: input}); err == nil {
		t.Fatal("expected in-place apply rejection")
	}
}

func TestPartialApplyRequiresKeepingIssueCellsOriginal(t *testing.T) {
	clients := fakeClients{byAccount: map[string]accountClient{
		"account-one": &fakeAccountClient{stores: []rtasales.Store{{BusinessID: "store-one"}}},
	}}
	applyCalls := 0
	engine := new(fakeEngine)
	engine.analyze = func(_ context.Context, _ xlsxfill.SalesProvider, request engineAnalyzeRequest) (*enginePlan, error) {
		return &enginePlan{PlanID: "partial-plan", InputPath: request.InputPath, Complete: true, ProblemCount: 1, ChangedCellCount: 2}, nil
	}
	engine.apply = func(_ context.Context, plan *enginePlan, _ string, allowPartial bool) (engineApplyResult, error) {
		applyCalls++
		if !allowPartial {
			return engineApplyResult{}, errors.New("expected partial apply")
		}
		return engineApplyResult{Complete: false, ProblemCount: plan.ProblemCount, ChangedCellCount: plan.ChangedCellCount, WroteWorkbook: true}, nil
	}
	app, _, _ := newTestApp(t, engine, clients)
	if _, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{DisplayName: "Primary", Account: "account-one", Password: "password-one", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	input := testWorkbook(t)
	analysis, err := app.Analyze(AnalyzeRequest{InputPath: input, Date: "2026-08-01", AllowPartial: true})
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "output.xlsx")
	if _, err := app.Apply(ApplyRequest{OperationID: analysis.OperationID, OutputPath: output, AllowPartial: true}); err == nil {
		t.Fatal("expected keepIssueOriginal safety rejection")
	}
	if applyCalls != 0 {
		t.Fatal("unsafe partial apply reached the engine")
	}
	result, err := app.Apply(ApplyRequest{
		OperationID: analysis.OperationID, OutputPath: output,
		AllowPartial: true, KeepIssueOriginal: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.WroteWorkbook || applyCalls != 1 {
		t.Fatalf("unexpected partial apply result: %#v, calls=%d", result, applyCalls)
	}
}

func TestAggregateProblemsCannotBePartiallyApplied(t *testing.T) {
	clients := fakeClients{byAccount: map[string]accountClient{
		"account-one": &fakeAccountClient{stores: []rtasales.Store{{BusinessID: "store-one"}}},
	}}
	applyCalls := 0
	engine := new(fakeEngine)
	engine.analyze = func(_ context.Context, _ xlsxfill.SalesProvider, request engineAnalyzeRequest) (*enginePlan, error) {
		return &enginePlan{
			PlanID: "aggregate-plan", InputPath: request.InputPath, Complete: true,
			ProblemCount: 1, AggregateProblemCount: 1, ChangedCellCount: 2,
		}, nil
	}
	engine.apply = func(context.Context, *enginePlan, string, bool) (engineApplyResult, error) {
		applyCalls++
		return engineApplyResult{}, nil
	}
	app, _, _ := newTestApp(t, engine, clients)
	if _, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{DisplayName: "Primary", Account: "account-one", Password: "password-one", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	analysis, err := app.Analyze(AnalyzeRequest{InputPath: testWorkbook(t), Date: "2026-08-01", AllowPartial: true})
	if err != nil {
		t.Fatal(err)
	}
	if analysis.AggregateProblemCount != 1 || analysis.IssueCount != 0 || analysis.CanApply {
		t.Fatalf("aggregate problem was not represented safely: %#v", analysis)
	}
	_, err = app.Apply(ApplyRequest{
		OperationID: analysis.OperationID, OutputPath: filepath.Join(t.TempDir(), "unsafe.xlsx"),
		AllowPartial: true, KeepIssueOriginal: true,
	})
	if err == nil || applyCalls != 0 {
		t.Fatalf("aggregate partial apply err=%v calls=%d", err, applyCalls)
	}
}

type cancelOnceAccountClient struct {
	started chan struct{}
	mu      sync.Mutex
	calls   int
}

func (c *cancelOnceAccountClient) Stores(context.Context) ([]rtasales.Store, error) {
	return []rtasales.Store{{BusinessID: "store-one"}}, nil
}

func (c *cancelOnceAccountClient) Sales(ctx context.Context, _ rtasales.SalesQuery) (*rtasales.SalesResult, error) {
	c.mu.Lock()
	c.calls++
	call := c.calls
	c.mu.Unlock()
	if call == 1 {
		close(c.started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	transactions := 3.0
	return &rtasales.SalesResult{
		TotalAmount: 128, TotalTransactionCount: &transactions,
		Items: []rtasales.SaleItem{{ArticleName: "test item"}},
	}, nil
}

func TestCancelPreservesPlanForRetryAndIncompleteCannotApply(t *testing.T) {
	client := &cancelOnceAccountClient{started: make(chan struct{})}
	app, _, _ := newTestApp(t, newXLSXEngine(), fakeClients{byAccount: map[string]accountClient{"account-one": client}})
	if _, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		DisplayName: "Primary", Account: "account-one", Password: "password-one", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	input := batchWorkbook(t)
	type analyzeResponse struct {
		result AnalysisResult
		err    error
	}
	done := make(chan analyzeResponse, 1)
	go func() {
		result, err := app.Analyze(AnalyzeRequest{InputPath: input, Date: "2026-08-01"})
		done <- analyzeResponse{result: result, err: err}
	}()
	<-client.started
	app.operationMu.Lock()
	operationID := app.active.id
	app.operationMu.Unlock()
	if err := app.Cancel(OperationRequest{OperationID: operationID}); err != nil {
		t.Fatal(err)
	}
	response := <-done
	if response.err != nil {
		t.Fatalf("cancelled analyze should return its resumable plan: %v", response.err)
	}
	if response.result.Complete || response.result.CanApply || response.result.RetryableCount != 1 {
		t.Fatalf("cancelled plan was incorrectly applyable: %#v", response.result)
	}
	if _, err := app.Apply(ApplyRequest{
		OperationID: operationID, OutputPath: filepath.Join(t.TempDir(), "unsafe.xlsx"),
	}); err == nil {
		t.Fatal("expected incomplete plan apply rejection")
	}
	retried, err := app.RetryFailed(OperationRequest{OperationID: operationID})
	if err != nil {
		t.Fatal(err)
	}
	if !retried.Complete || !retried.CanApply || retried.ChangedCellCount != 2 {
		t.Fatalf("retry did not complete pending work: %#v", retried)
	}
}

type oneAccountManyStoresClient struct {
	stores   []rtasales.Store
	failID   string
	failErr  error
	mu       sync.Mutex
	failures int
	queries  []string
}

func (c *oneAccountManyStoresClient) Stores(context.Context) ([]rtasales.Store, error) {
	return append([]rtasales.Store(nil), c.stores...), nil
}

func (c *oneAccountManyStoresClient) Sales(_ context.Context, query rtasales.SalesQuery) (*rtasales.SalesResult, error) {
	c.mu.Lock()
	c.queries = append(c.queries, query.BusinessStoreID)
	if query.BusinessStoreID == c.failID && c.failures < 3 {
		c.failures++
		err := c.failErr
		c.mu.Unlock()
		return nil, err
	}
	c.mu.Unlock()
	transactions := 4.0
	return &rtasales.SalesResult{
		TotalAmount: 40, TotalTransactionCount: &transactions,
		Items: []rtasales.SaleItem{{Matnr: "ITEM"}},
	}, nil
}

func TestAnalyzeOneProfileManyStoresKeepsRetryableFailureAsQueryFailed(t *testing.T) {
	const storeCount = 16
	failID := "S03"
	stores := make([]rtasales.Store, storeCount)
	date := time.Date(2026, 8, 1, 0, 0, 0, 0, time.Local)
	client := &oneAccountManyStoresClient{
		failID:  failID,
		failErr: &rtasales.UpstreamError{Operation: "sales", StatusCode: 429, Body: "too many requests"},
	}
	for index := 0; index < storeCount; index++ {
		storeID := "S" + twoDigit(index+1)
		stores[index] = rtasales.Store{BusinessID: storeID, Label: storeID}
	}
	client.stores = stores
	app, _, _ := newTestApp(t, newXLSXEngine(), fakeClients{byAccount: map[string]accountClient{"account-one": client}})
	if _, err := app.CreateOrUpdateProfile(ProfileUpsertRequest{
		DisplayName: "Primary", Account: "account-one", Password: "password-one", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	input := multiStoreWorkbook(t, stores, date)
	result, err := app.Analyze(AnalyzeRequest{InputPath: input, Date: "2026-08-01"})
	if err != nil {
		t.Fatal(err)
	}
	if result.RetryableCount != 1 || result.FailedCount != 1 {
		t.Fatalf("retryable/failed=%d/%d, want 1/1: %#v", result.RetryableCount, result.FailedCount, result)
	}
	failed := 0
	succeeded := 0
	for _, row := range result.Preview {
		if permissionIssue(row.Message) {
			t.Fatalf("listed store marked as permission: %#v", row)
		}
		if row.Status == "failed" {
			if row.Message != "query_failed" || row.WorkbookStoreID != failID {
				t.Fatalf("failed row was not retryable query_failed: %#v", row)
			}
			failed++
			continue
		}
		if row.Status != "change" {
			t.Fatalf("other listed store did not succeed: %#v", row)
		}
		succeeded++
	}
	if failed != 1 || succeeded != storeCount-1 {
		t.Fatalf("failed=%d succeeded=%d", failed, succeeded)
	}
	for _, issue := range result.Issues {
		if permissionIssue(issue.Message) {
			t.Fatalf("listed store issue marked as permission: %#v", issue)
		}
		if issue.Message == "query_failed" && !issue.Retryable {
			t.Fatal("query_failed must stay retryable")
		}
	}
}

func twoDigit(value int) string {
	if value < 10 {
		return "0" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}

func permissionIssue(message string) bool {
	switch message {
	case "store_not_authorized", "no_authorized_store_match":
		return true
	}
	lower := strings.ToLower(message)
	return strings.Contains(lower, "not authorized") || strings.Contains(lower, "permission")
}

func multiStoreWorkbook(t *testing.T, stores []rtasales.Store, date time.Time) string {
	t.Helper()
	book := excelize.NewFile()
	defer func() { _ = book.Close() }()
	if err := book.SetSheetName("Sheet1", xlsxfill.DefaultSheetName); err != nil {
		t.Fatal(err)
	}
	for index, store := range stores {
		row := index + 2
		for cell, value := range map[string]any{
			"C" + strconv.Itoa(row): store.BusinessID,
			"E" + strconv.Itoa(row): store.Label,
			"F" + strconv.Itoa(row): date,
		} {
			if err := book.SetCellValue(xlsxfill.DefaultSheetName, cell, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	path := filepath.Join(t.TempDir(), "many-stores.xlsx")
	if err := book.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func batchWorkbook(t *testing.T) string {
	t.Helper()
	book := excelize.NewFile()
	defer func() { _ = book.Close() }()
	if err := book.SetSheetName("Sheet1", xlsxfill.DefaultSheetName); err != nil {
		t.Fatal(err)
	}
	for cell, value := range map[string]any{
		"C2": "store-one", "E2": "Store", "F2": time.Date(2026, 8, 1, 0, 0, 0, 0, time.Local),
	} {
		if err := book.SetCellValue(xlsxfill.DefaultSheetName, cell, value); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "batch.xlsx")
	if err := book.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSaveWorkbookUsesDatedNonInPlaceDefault(t *testing.T) {
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{byAccount: map[string]accountClient{}})
	input := testWorkbook(t)
	dialogs := app.dialogs.(*fakeDialogs)
	dialogs.save = filepath.Join(filepath.Dir(input), "chosen.xlsx")
	selected, err := app.SaveWorkbook(SaveWorkbookRequest{InputPath: input, Date: "2026-08-15"})
	if err != nil {
		t.Fatal(err)
	}
	if selected != dialogs.save || dialogs.lastSave.DefaultFilename != "input_filled_20260815.xlsx" {
		t.Fatalf("unexpected save dialog result: %q %#v", selected, dialogs.lastSave)
	}
	dialogs.save = filepath.Join(filepath.Dir(input), "range.xlsx")
	if _, err := app.SaveWorkbook(SaveWorkbookRequest{
		InputPath: input, From: "2026-08-01", To: "2026-08-15",
	}); err != nil {
		t.Fatal(err)
	}
	if dialogs.lastSave.DefaultFilename != "input_filled_20260801-20260815.xlsx" {
		t.Fatalf("date range missing from default output name: %#v", dialogs.lastSave)
	}
}
