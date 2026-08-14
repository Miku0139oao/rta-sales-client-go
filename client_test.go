package rtasales

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type solverFunc func(context.Context, []byte) (string, error)

func (solver solverFunc) Solve(ctx context.Context, image []byte) (string, error) {
	return solver(ctx, image)
}

type rtaFixture struct {
	testing          *testing.T
	server           *httptest.Server
	loginSubmissions atomic.Int32
	captchaRequests  atomic.Int32
	areaRequests     atomic.Int32
	salesRequests    atomic.Int32
	mu               sync.Mutex
	payloads         []salesQueryPayload
	failPage         int
	expireNextSales  bool
	areaEmpty        bool
}

func newRTAFixture(t *testing.T) *rtaFixture {
	t.Helper()
	fixture := &rtaFixture{testing: t}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (f *rtaFixture) serveHTTP(response http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case "/getVerifyCodeImg":
		f.captchaRequests.Add(1)
		http.SetCookie(response, &http.Cookie{Name: "challenge", Value: "active", Path: "/"})
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write([]byte("fake-image"))
	case "/doLogin":
		f.loginSubmissions.Add(1)
		if request.URL.Query().Get("password") != passwordDigest("secret") {
			writeJSON(response, map[string]any{"code": "1001", "msg": "bad password"})
			return
		}
		if request.URL.Query().Get("verifyCode") != "RIGHT" {
			writeJSON(response, map[string]any{"code": "2020350002", "msg": "驗証碼錯誤"})
			return
		}
		http.SetCookie(response, &http.Cookie{
			Name:    "sid",
			Value:   "valid",
			Path:    "/",
			Expires: time.Now().Add(24 * time.Hour),
		})
		writeJSON(response, map[string]any{"code": "0000"})
	case "/storeStock/areasAndStores":
		f.areaRequests.Add(1)
		if !hasCookie(request, "sid", "valid") {
			writeJSON(response, map[string]any{"code": "9800", "msg": "用户未登录"})
			return
		}
		f.mu.Lock()
		areaEmpty := f.areaEmpty
		f.mu.Unlock()
		storeTree := []any{map[string]any{"label": "STOREA-Main Store", "storeId": "UPSTREAM-A", "value": "UPSTREAM-A"}}
		if areaEmpty {
			storeTree = nil
		}
		writeJSON(response, map[string]any{
			"code": "0000",
			"data": map[string]any{
				"storeTree": storeTree,
			},
		})
	case "/storeStock/allStore":
		if !hasCookie(request, "sid", "valid") {
			writeJSON(response, map[string]any{"code": "9800", "msg": "用户未登录"})
			return
		}
		writeJSON(response, map[string]any{
			"code": "0000",
			"data": []any{map[string]any{"label": "STOREB-Flat Store", "storeId": "UPSTREAM-B", "value": "UPSTREAM-B"}},
		})
	case "/open/data/query":
		f.salesRequests.Add(1)
		if !hasCookie(request, "sid", "valid") {
			writeJSON(response, map[string]any{"code": "9800", "msg": "用户未登录"})
			return
		}
		f.mu.Lock()
		if f.expireNextSales {
			f.expireNextSales = false
			f.mu.Unlock()
			http.SetCookie(response, &http.Cookie{Name: "sid", Value: "expired", Path: "/"})
			writeJSON(response, map[string]any{"code": "9800", "msg": "用户未登录"})
			return
		}
		f.mu.Unlock()
		if err := request.ParseForm(); err != nil {
			f.testing.Errorf("parse sales form: %v", err)
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		page, _ := strconv.Atoi(request.Form.Get("pageNum"))
		if page == f.failPage {
			http.Error(response, "page failed", http.StatusBadGateway)
			return
		}
		var payload salesQueryPayload
		if err := json.Unmarshal([]byte(request.Form.Get("queryParam")), &payload); err != nil {
			f.testing.Errorf("decode sales query: %v", err)
		}
		f.mu.Lock()
		f.payloads = append(f.payloads, payload)
		f.mu.Unlock()
		row := map[string]any{
			"purchase_category4_name": "HEALTH",
			"purchase_category5_name": "VITAMINS",
			"matnr":                   fmt.Sprintf("SKU-%d", page),
			"article_name":            fmt.Sprintf("Item %d", page),
			"tp_sale_amount":          fmt.Sprintf("%d.5", page*10),
			"tp_gross_sale_qty":       page,
		}
		writeJSON(response, map[string]any{
			"code": "0000",
			"data": map[string]any{
				"countResult":   map[string]any{"result": []any{map[string]any{"counts": 1001}}},
				"executeResult": map[string]any{"result": []any{row}},
			},
		})
	default:
		http.NotFound(response, request)
	}
}

func (f *rtaFixture) client(t *testing.T, cookieFile string) (*Client, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	var ocrCalls atomic.Int32
	var fallbackCalls atomic.Int32
	client, err := NewClient(Config{
		Account:    "account",
		Password:   "secret",
		CookieFile: cookieFile,
		CaptchaSolvers: []CaptchaSolver{
			solverFunc(func(context.Context, []byte) (string, error) {
				ocrCalls.Add(1)
				return "WRONG", nil
			}),
			solverFunc(func(context.Context, []byte) (string, error) {
				fallbackCalls.Add(1)
				return "RIGHT", nil
			}),
		},
		PageConcurrency: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	client.endpoints = endpoints{sso: f.server.URL, stock: f.server.URL, dsa: f.server.URL}
	return client, &ocrCalls, &fallbackCalls
}

func TestSalesResolvesStorePaginatesAndAggregates(t *testing.T) {
	fixture := newRTAFixture(t)
	client, ocrCalls, fallbackCalls := fixture.client(t, "")
	result, err := client.Sales(context.Background(), SalesQuery{
		BusinessStoreID: "STOREA",
		StartDate:       time.Date(2026, 6, 25, 23, 59, 0, 0, time.FixedZone("test", 9*60*60)),
		EndDate:         time.Date(2026, 6, 26, 1, 0, 0, 0, time.FixedZone("test", 9*60*60)),
		Category:        "HA",
		ItemCodes:       []string{"SKU-A", "SKU-A", " SKU-B "},
	})
	if err != nil {
		t.Fatal(err)
	}
	if fixture.loginSubmissions.Load() != 2 || ocrCalls.Load() != 1 || fallbackCalls.Load() != 1 {
		t.Fatalf("login submissions=%d OCR=%d fallback=%d, want 2/1/1", fixture.loginSubmissions.Load(), ocrCalls.Load(), fallbackCalls.Load())
	}
	if result.Store.BusinessID != "STOREA" || result.Store.Label != "STOREA-Main Store" {
		t.Fatalf("unexpected store: %+v", result.Store)
	}
	storeJSON, err := json.Marshal(result.Store)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(storeJSON), `{"business_id":"STOREA","label":"STOREA-Main Store"}`; got != want {
		t.Fatalf("public store JSON=%s, want %s", got, want)
	}
	if result.StartDate != "2026-06-25" || result.EndDate != "2026-06-26" {
		t.Fatalf("dates were timezone-converted: %s to %s", result.StartDate, result.EndDate)
	}
	if len(result.Items) != 2 || result.Items[0].Matnr != "SKU-1" || result.Items[1].Matnr != "SKU-2" {
		t.Fatalf("pages were not merged in order: %+v", result.Items)
	}
	if result.TotalAmount != 31 || result.GrossQuantity != 3 {
		t.Fatalf("aggregate total=%v quantity=%v, want 31/3", result.TotalAmount, result.GrossQuantity)
	}
	if len(result.Categories) != 1 || len(result.Categories[0].Items) != 2 {
		t.Fatalf("unexpected category aggregate: %+v", result.Categories)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.payloads) != 2 {
		t.Fatalf("payload count=%d, want 2", len(fixture.payloads))
	}
	for _, payload := range fixture.payloads {
		if payload.StoreID != "UPSTREAM-A" || payload.StoreIDString != "UPSTREAM-A" {
			t.Fatalf("sales payload did not use resolved store: %+v", payload)
		}
		if payload.Matnr != "SKU-A,SKU-B" || payload.MatnrString != "SKU-A,SKU-B" {
			t.Fatalf("unexpected item code filter: %+v", payload)
		}
	}
}

func TestEmbeddedOCRFailureUsesConfiguredFallback(t *testing.T) {
	fixture := newRTAFixture(t)
	var fallbackCalls atomic.Int32
	client, err := NewClient(Config{
		Account:  "account",
		Password: "secret",
		CaptchaSolvers: []CaptchaSolver{
			NewEmbeddedOCRSolver(EmbeddedOCRConfig{}),
			solverFunc(func(context.Context, []byte) (string, error) {
				fallbackCalls.Add(1)
				return "RIGHT", nil
			}),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	client.endpoints = endpoints{sso: fixture.server.URL, stock: fixture.server.URL, dsa: fixture.server.URL}
	stores, err := client.Stores(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(stores) != 1 || fallbackCalls.Load() != 1 || fixture.loginSubmissions.Load() != 1 {
		t.Fatalf("stores=%d fallback calls=%d login submissions=%d, want 1/1/1", len(stores), fallbackCalls.Load(), fixture.loginSubmissions.Load())
	}
}

func TestLoginAttemptsRequestFreshCaptchas(t *testing.T) {
	tests := []struct {
		name       string
		configured int
		want       int32
	}{
		{name: "default", want: 4},
		{name: "configured", configured: 2, want: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRTAFixture(t)
			var solverCalls atomic.Int32
			client, err := NewClient(Config{
				Account:       "account",
				Password:      "secret",
				LoginAttempts: test.configured,
				CaptchaSolvers: []CaptchaSolver{solverFunc(func(context.Context, []byte) (string, error) {
					solverCalls.Add(1)
					return "", errors.New("uncertain captcha")
				})},
			})
			if err != nil {
				t.Fatal(err)
			}
			client.endpoints = endpoints{sso: fixture.server.URL, stock: fixture.server.URL, dsa: fixture.server.URL}
			err = client.login(context.Background())
			var captchaError *CaptchaError
			if !errors.As(err, &captchaError) {
				t.Fatalf("error=%T %v, want CaptchaError", err, err)
			}
			if fixture.captchaRequests.Load() != test.want || solverCalls.Load() != test.want {
				t.Fatalf("captcha requests=%d solver calls=%d, want %d/%d", fixture.captchaRequests.Load(), solverCalls.Load(), test.want, test.want)
			}
			if fixture.loginSubmissions.Load() != 0 {
				t.Fatalf("uncertain captcha was submitted %d times", fixture.loginSubmissions.Load())
			}
		})
	}
}

func TestNewClientRejectsExcessiveLoginAttempts(t *testing.T) {
	_, err := NewClient(Config{
		Account:         "account",
		Password:        "secret",
		LoginAttempts:   maximumLoginAttempts + 1,
		CaptchaSolvers:  []CaptchaSolver{solverFunc(func(context.Context, []byte) (string, error) { return "RIGHT", nil })},
		PageConcurrency: 1,
	})
	var input *InputError
	if !errors.As(err, &input) || input.Field != "LoginAttempts" {
		t.Fatalf("error=%T %v, want LoginAttempts InputError", err, err)
	}
}

func TestSavedCookiesAvoidAnotherLogin(t *testing.T) {
	fixture := newRTAFixture(t)
	cookieFile := filepath.Join(t.TempDir(), "rta.cookies.json")
	first, _, _ := fixture.client(t, cookieFile)
	if _, err := first.Stores(context.Background()); err != nil {
		t.Fatal(err)
	}
	loginCount := fixture.loginSubmissions.Load()
	second, _, _ := fixture.client(t, cookieFile)
	if _, err := second.Stores(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fixture.loginSubmissions.Load() != loginCount {
		t.Fatalf("saved cookie was ignored; login submissions changed from %d to %d", loginCount, fixture.loginSubmissions.Load())
	}
}

func TestExpiredSalesSessionAutomaticallyRelogs(t *testing.T) {
	fixture := newRTAFixture(t)
	client, _, _ := fixture.client(t, "")
	if _, err := client.Stores(context.Background()); err != nil {
		t.Fatal(err)
	}
	fixture.mu.Lock()
	fixture.expireNextSales = true
	fixture.mu.Unlock()
	before := fixture.loginSubmissions.Load()
	_, err := client.Sales(context.Background(), SalesQuery{
		BusinessStoreID: "STOREA",
		StartDate:       time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC),
		EndDate:         time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if fixture.loginSubmissions.Load() <= before {
		t.Fatal("expired sales session did not trigger automatic login")
	}
}

func TestSalesFailsInsteadOfReturningPartialPages(t *testing.T) {
	fixture := newRTAFixture(t)
	fixture.failPage = 2
	client, _, _ := fixture.client(t, "")
	result, err := client.Sales(context.Background(), SalesQuery{
		BusinessStoreID: "STOREA",
		StartDate:       time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC),
		EndDate:         time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC),
	})
	if err == nil || result != nil {
		t.Fatalf("expected whole-query failure, result=%+v err=%v", result, err)
	}
	var upstream *UpstreamError
	if !errors.As(err, &upstream) || upstream.StatusCode != http.StatusBadGateway {
		t.Fatalf("unexpected error: %T %v", err, err)
	}
}

func TestSalesValidatesDateBeforeNetwork(t *testing.T) {
	client, err := NewClient(Config{
		Account:        "account",
		Password:       "secret",
		CaptchaSolvers: []CaptchaSolver{solverFunc(func(context.Context, []byte) (string, error) { return "RIGHT", nil })},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Sales(context.Background(), SalesQuery{
		BusinessStoreID: "STOREA",
		StartDate:       time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC),
		EndDate:         time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC),
	})
	var input *InputError
	if !errors.As(err, &input) || input.Field != "EndDate" {
		t.Fatalf("unexpected error: %T %v", err, err)
	}
}

func TestValidateStoreRecordsRejectsConflictingBusinessIDs(t *testing.T) {
	_, err := validateStoreRecords([]storeRecord{
		{Store: Store{BusinessID: "STOREA"}, upstreamID: "UPSTREAM-A"},
		{Store: Store{BusinessID: "STOREA"}, upstreamID: "UPSTREAM-B"},
	})
	var protocol *ProtocolError
	if !errors.As(err, &protocol) {
		t.Fatalf("unexpected error: %T %v", err, err)
	}
}

func TestStoresFallsBackToFlatList(t *testing.T) {
	fixture := newRTAFixture(t)
	fixture.areaEmpty = true
	client, _, _ := fixture.client(t, "")
	stores, err := client.Stores(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(stores) != 1 || stores[0].BusinessID != "STOREB" || stores[0].Label != "STOREB-Flat Store" {
		t.Fatalf("unexpected fallback stores: %+v", stores)
	}
}

func TestStoreLookupIsExact(t *testing.T) {
	fixture := newRTAFixture(t)
	client, _, _ := fixture.client(t, "")
	if _, err := client.Stores(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err := client.Sales(context.Background(), SalesQuery{
		BusinessStoreID: "STORE",
		StartDate:       time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC),
		EndDate:         time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC),
	})
	var missing *StoreNotFoundError
	if !errors.As(err, &missing) || missing.BusinessStoreID != "STORE" {
		t.Fatalf("unexpected error: %T %v", err, err)
	}
}

func TestConcurrentStoresShareInitialLoadAndLogin(t *testing.T) {
	fixture := newRTAFixture(t)
	client, _, _ := fixture.client(t, "")
	const callers = 12
	errorsFound := make(chan error, callers)
	var waitGroup sync.WaitGroup
	for index := 0; index < callers; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			stores, err := client.Stores(context.Background())
			if err == nil && len(stores) != 1 {
				err = fmt.Errorf("store count=%d", len(stores))
			}
			errorsFound <- err
		}()
	}
	waitGroup.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatal(err)
		}
	}
	if fixture.areaRequests.Load() != 2 {
		t.Fatalf("area requests=%d, want one unauthenticated request plus one retry", fixture.areaRequests.Load())
	}
	if fixture.loginSubmissions.Load() != 2 {
		t.Fatalf("login submissions=%d, want one OCR rejection plus one fallback", fixture.loginSubmissions.Load())
	}
}

func writeJSON(response http.ResponseWriter, payload any) {
	response.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(response).Encode(payload)
}

func hasCookie(request *http.Request, name, value string) bool {
	cookie, err := request.Cookie(name)
	return err == nil && cookie.Value == value
}
