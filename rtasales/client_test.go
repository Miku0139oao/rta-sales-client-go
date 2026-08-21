package rtasales

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
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
	testing             *testing.T
	server              *httptest.Server
	loginSubmissions    atomic.Int32
	captchaRequests     atomic.Int32
	salesRequests       atomic.Int32
	transactionRequests atomic.Int32
	storeRequests       atomic.Int32
	mu                  sync.Mutex
	payloads            []salesQueryPayload
	rawPayloads         []map[string]any
	transactionPayloads []trendTransactionQueryPayload
	transactionForms    []url.Values
	failPage            int
	expireNextSales     bool
	salesFailRemaining  int
	salesFailStatus     int
	salesRetryAfter     string
	salesFailBody       []byte
	salesInFlight       int
	salesMaxInFlight    int
	salesHold           time.Duration
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
	case "/open/data/query":
		f.salesRequests.Add(1)
		f.enterSales()
		defer f.leaveSales()
		f.mu.Lock()
		hold := f.salesHold
		f.mu.Unlock()
		if hold > 0 {
			time.Sleep(hold)
		}
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
		if f.salesFailRemaining > 0 {
			f.salesFailRemaining--
			status := f.salesFailStatus
			if status == 0 {
				status = http.StatusTooManyRequests
			}
			retryAfter := f.salesRetryAfter
			body := append([]byte(nil), f.salesFailBody...)
			f.mu.Unlock()
			if retryAfter != "" {
				response.Header().Set("Retry-After", retryAfter)
			}
			if len(body) > 0 {
				response.Header().Set("Content-Type", "application/json")
				response.WriteHeader(status)
				_, _ = response.Write(body)
				return
			}
			http.Error(response, "temporary failure", status)
			return
		}
		f.mu.Unlock()
		if err := request.ParseForm(); err != nil {
			f.testing.Errorf("parse sales form: %v", err)
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		for _, column := range []string{
			"column_purchase_category1_code", "column_purchase_category2_code", "column_purchase_category3_code",
			"column_purchase_category4_code", "column_purchase_category5_code", "brand_name",
		} {
			if !strings.Contains(request.Form.Get("showColumns"), `"`+column+`":true`) || !strings.Contains(request.Form.Get("columnSeq"), column) {
				f.testing.Errorf("sales form did not request %s", column)
			}
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
		if days := inclusiveDaysYYYYMMDD(payload.CurrentStartDay, payload.CurrentEndDay); days > salesMaxInclusiveDays {
			f.testing.Errorf("sales query exceeds %d days: %s to %s (%d days)", salesMaxInclusiveDays, payload.CurrentStartDay, payload.CurrentEndDay, days)
			http.Error(response, "date range too long", http.StatusGatewayTimeout)
			return
		}
		var rawPayload map[string]any
		if err := json.Unmarshal([]byte(request.Form.Get("queryParam")), &rawPayload); err != nil {
			f.testing.Errorf("decode raw sales query: %v", err)
		}
		f.mu.Lock()
		f.payloads = append(f.payloads, payload)
		f.rawPayloads = append(f.rawPayloads, rawPayload)
		f.mu.Unlock()
		row := map[string]any{
			"purchase_category1_name":        "HEALTH & BEAUTY",
			"column_purchase_category1_code": "A",
			"purchase_category2_name":        "HEALTH CARE",
			"column_purchase_category2_code": "A01",
			"purchase_category3_name":        "SUPPLEMENTS",
			"column_purchase_category3_code": "A0101",
			"purchase_category4_name":        "HEALTH",
			"column_purchase_category4_code": "A010101",
			"purchase_category5_name":        "VITAMINS",
			"column_purchase_category5_code": "A01010101",
			"matnr":                          fmt.Sprintf("SKU-%d", page),
			"article_name":                   fmt.Sprintf("Item %d", page),
			"brand_name":                     "Fixture Brand",
			"tp_transaction_count":           page * 3,
			"tp_transaction_count_agg":       "999",
			"tp_sale_amount":                 fmt.Sprintf("%d.5", page*10),
			"tp_gross_sale_qty":              page,
		}
		writeJSON(response, map[string]any{
			"code": "0000",
			"data": map[string]any{
				"countResult":   map[string]any{"result": []any{map[string]any{"counts": 1001, "tp_transaction_count": "9999"}}},
				"executeResult": map[string]any{"result": []any{row}},
			},
		})
	case "/data/pc/v1/query":
		f.transactionRequests.Add(1)
		if !hasCookie(request, "sid", "valid") {
			writeJSON(response, map[string]any{"code": "9800", "msg": "用户未登录"})
			return
		}
		if request.Method != http.MethodPost || request.Header.Get("Accept") != "application/json" || request.Header.Get("Origin") != "https://partner.rta-os.com" || request.Header.Get("Referer") != "https://partner.rta-os.com/" {
			f.testing.Errorf("unexpected Trend View request metadata")
		}
		if err := request.ParseForm(); err != nil {
			f.testing.Errorf("parse Trend View form: %v", err)
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		var payload trendTransactionQueryPayload
		if err := json.Unmarshal([]byte(request.Form.Get("queryParam")), &payload); err != nil {
			f.testing.Errorf("decode Trend View query: %v", err)
		}
		if days := inclusiveDaysISODate(payload.CurrentStart, payload.CurrentEnd); days > salesMaxInclusiveDays {
			f.testing.Errorf("Trend View query exceeds %d days: %s to %s (%d days)", salesMaxInclusiveDays, payload.CurrentStart, payload.CurrentEnd, days)
			http.Error(response, "date range too long", http.StatusGatewayTimeout)
			return
		}
		formCopy := make(url.Values, len(request.Form))
		for key, values := range request.Form {
			formCopy[key] = append([]string(nil), values...)
		}
		f.mu.Lock()
		f.transactionPayloads = append(f.transactionPayloads, payload)
		f.transactionForms = append(f.transactionForms, formCopy)
		f.mu.Unlock()
		writeJSON(response, map[string]any{
			"code": 0,
			"data": map[string]any{
				"count": 4,
				"data": []any{
					map[string]any{"show_date": "25-06-2026", "group_sales_ticket_num": "120", "gross_sales_gross_sale_untaxed_amt": "100.25"},
					map[string]any{"show_date": "26-06-2026", "group_sales_ticket_num": 311, "gross_sales_gross_sale_untaxed_amt": 200.5},
					map[string]any{"show_date": "27-06-2026", "group_sales_ticket_num": "4000", "gross_sales_gross_sale_untaxed_amt": 5000},
					map[string]any{"show_date": "", "group_sales_ticket_num": 431, "gross_sales_gross_sale_untaxed_amt": 300.75},
				},
			},
		})
	case "/appQuery/listStore":
		f.storeRequests.Add(1)
		if !hasCookie(request, "sid", "valid") {
			writeJSON(response, map[string]any{"code": "9800", "msg": "用户未登录"})
			return
		}
		if request.Method != http.MethodGet || request.Header.Get("Origin") != "https://partner.rta-os.com" || request.Header.Get("Referer") != "https://partner.rta-os.com/" {
			f.testing.Errorf("unexpected authorized-store request metadata")
		}
		writeJSON(response, map[string]any{
			"code": "success",
			"data": []any{
				map[string]any{"key": "INTERNAL-A", "value": "STOREA-Fixture Store A"},
				map[string]any{"key": 2002, "value": "STOREB-Fixture Store B"},
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
	client.endpoints = endpoints{sso: f.server.URL, dsa: f.server.URL, cockpit: f.server.URL, authStores: f.server.URL}
	client.retryWait = func(ctx context.Context, _ time.Duration) error {
		return ctx.Err()
	}
	return client, &ocrCalls, &fallbackCalls
}

func (f *rtaFixture) enterSales() {
	f.mu.Lock()
	f.salesInFlight++
	if f.salesInFlight > f.salesMaxInFlight {
		f.salesMaxInFlight = f.salesInFlight
	}
	f.mu.Unlock()
}

func (f *rtaFixture) leaveSales() {
	f.mu.Lock()
	f.salesInFlight--
	f.mu.Unlock()
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
	if fixture.transactionRequests.Load() != 1 {
		t.Fatalf("Trend View requests=%d, want 1", fixture.transactionRequests.Load())
	}
	if result.Store.BusinessID != "STOREA" || result.Store.Label != "STOREA-Fixture Store A" {
		t.Fatalf("unexpected store: %+v", result.Store)
	}
	storeJSON, err := json.Marshal(result.Store)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(storeJSON), `{"business_id":"STOREA","label":"STOREA-Fixture Store A"}`; got != want {
		t.Fatalf("public store JSON=%s, want %s", got, want)
	}
	if result.StartDate != "2026-06-25" || result.EndDate != "2026-06-26" {
		t.Fatalf("dates were timezone-converted: %s to %s", result.StartDate, result.EndDate)
	}
	if len(result.Items) != 2 || result.Items[0].Matnr != "SKU-1" || result.Items[1].Matnr != "SKU-2" {
		t.Fatalf("pages were not merged in order: %+v", result.Items)
	}
	if result.Items[0].PurchaseCategory1Code != "A" || result.Items[0].PurchaseCategory5Code != "A01010101" || result.Items[0].BrandName != "Fixture Brand" {
		t.Fatalf("category codes or brand were not decoded: %+v", result.Items[0])
	}
	if result.TotalAmount != 31 || result.GrossQuantity != 3 {
		t.Fatalf("aggregate total=%v quantity=%v, want 31/3", result.TotalAmount, result.GrossQuantity)
	}
	if result.TrendGrossSaleAmount == nil || *result.TrendGrossSaleAmount != 300.75 {
		t.Fatalf("Trend View gross sales total=%v, want 300.75", result.TrendGrossSaleAmount)
	}
	if result.TotalTransactionCount == nil || *result.TotalTransactionCount != 431 {
		t.Fatalf("Trend View transaction total=%v, want 431", result.TotalTransactionCount)
	}
	if len(result.Categories) != 1 || len(result.Categories[0].Items) != 2 {
		t.Fatalf("unexpected category aggregate: %+v", result.Categories)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.payloads) != 2 {
		t.Fatalf("payload count=%d, want 2", len(fixture.payloads))
	}
	for index, payload := range fixture.payloads {
		if payload.StoreID != "INTERNAL-A" || payload.StoreIDString != "STOREA-Fixture Store A" {
			t.Fatalf("sales payload has wrong authorized-store filter: %+v", payload)
		}
		if payload.Matnr != "SKU-A,SKU-B" || payload.MatnrString != "SKU-A,SKU-B" {
			t.Fatalf("unexpected item code filter: %+v", payload)
		}
		for key, want := range map[string]string{
			"store_id": "INTERNAL-A", "store_id_str": "STOREA-Fixture Store A",
			"store_cluster_code": "", "large_area_code": "",
		} {
			if value, exists := fixture.rawPayloads[index][key]; !exists || value != want {
				t.Fatalf("query field %q=%v exists=%t, want %q", key, value, exists, want)
			}
		}
	}
	if len(fixture.transactionPayloads) != 1 || len(fixture.transactionForms) != 1 {
		t.Fatalf("Trend View payloads=%d forms=%d, want 1/1", len(fixture.transactionPayloads), len(fixture.transactionForms))
	}
	transactionPayload := fixture.transactionPayloads[0]
	if transactionPayload.SiteCode != "STOREA" || transactionPayload.CurrentStart != "2026-06-15" || transactionPayload.CurrentEnd != "2026-06-26" || transactionPayload.DateType != "1" || transactionPayload.CalendarUnit != "one" {
		t.Fatalf("unexpected Trend View query payload: %+v", transactionPayload)
	}
	if len(result.TrendDays) != 2 || result.TrendDays[0].Date != "2026-06-25" || result.TrendDays[1].Date != "2026-06-26" {
		t.Fatalf("Trend View days=%+v, want 25 and 26 June", result.TrendDays)
	}
	transactionForm := fixture.transactionForms[0]
	for key, want := range map[string]string{
		"pageCode":    "storeRealTimeSalesMannings",
		"moduleCode":  "trendTable",
		"tabCode":     "trend",
		"serviceCode": "achievement",
		"dataCode":    "storeRealTimeSalesMannings.trendTable",
		"filterParam": "{}",
		"pageNum":     "1",
		"pageSize":    "50",
	} {
		if got := transactionForm.Get(key); got != want {
			t.Fatalf("Trend View form %q=%q, want %q", key, got, want)
		}
	}
	var columns []string
	if err := json.Unmarshal([]byte(transactionForm.Get("showColumns")), &columns); err != nil {
		t.Fatalf("decode Trend View columns: %v", err)
	}
	if !containsString(columns, "show_date") || !containsString(columns, "group_sales_ticket_num") || !containsString(columns, "gross_sales_gross_sale_untaxed_amt") {
		t.Fatalf("Trend View columns omit date, transaction, or gross sales fields: %v", columns)
	}
}

func TestCompactSalesOmitsRawRowsAndCategoryItemCopies(t *testing.T) {
	fixture := newRTAFixture(t)
	client, _, _ := fixture.client(t, "")
	result, err := client.Sales(context.Background(), SalesQuery{
		BusinessStoreID: "STOREA",
		StartDate:       time.Date(2026, 6, 25, 0, 0, 0, 0, time.Local),
		EndDate:         time.Date(2026, 6, 25, 0, 0, 0, 0, time.Local),
		SkipTrend:       true,
		Compact:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) == 0 {
		t.Fatal("compact sales dropped article rows")
	}
	for _, item := range result.Items {
		if item.Raw != nil {
			t.Fatal("compact sales kept a raw row map")
		}
	}
	for _, category := range result.Categories {
		if len(category.Items) != 0 {
			t.Fatalf("compact sales copied %d category items", len(category.Items))
		}
	}
}

func TestSalesCanSkipWholeStoreTrendRequest(t *testing.T) {
	fixture := newRTAFixture(t)
	client, _, _ := fixture.client(t, "")
	result, err := client.Sales(context.Background(), SalesQuery{
		BusinessStoreID: "STOREA",
		StartDate:       time.Date(2026, 6, 25, 0, 0, 0, 0, time.Local),
		EndDate:         time.Date(2026, 6, 25, 0, 0, 0, 0, time.Local),
		SkipTrend:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if fixture.transactionRequests.Load() != 0 {
		t.Fatalf("Trend View requests=%d, want 0", fixture.transactionRequests.Load())
	}
	if result.TrendGrossSaleAmount != nil || result.TotalTransactionCount != nil {
		t.Fatalf("skipped Trend View returned totals: amount=%v transactions=%v", result.TrendGrossSaleAmount, result.TotalTransactionCount)
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
	client.endpoints = endpoints{sso: fixture.server.URL, dsa: fixture.server.URL, cockpit: fixture.server.URL, authStores: fixture.server.URL}
	result, err := client.Sales(context.Background(), SalesQuery{
		BusinessStoreID: "STOREA",
		StartDate:       time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC),
		EndDate:         time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) == 0 || fallbackCalls.Load() != 1 || fixture.loginSubmissions.Load() != 1 {
		t.Fatalf("items=%d fallback calls=%d login submissions=%d, want nonzero/1/1", len(result.Items), fallbackCalls.Load(), fixture.loginSubmissions.Load())
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
			client.endpoints = endpoints{sso: fixture.server.URL, dsa: fixture.server.URL, authStores: fixture.server.URL}
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

func TestNewClientDoesNotRequireBusinessStoreBinding(t *testing.T) {
	client, err := NewClient(Config{
		Account:        "account",
		Password:       "secret",
		CaptchaSolvers: []CaptchaSolver{solverFunc(func(context.Context, []byte) (string, error) { return "RIGHT", nil })},
	})
	if err != nil || client == nil {
		t.Fatalf("client=%v error=%v, want a configured account client", client, err)
	}
}

func TestDefaultHTTPTransportKeepsEnoughSocketsFor160StoreJobs(t *testing.T) {
	transport := defaultHTTPTransport()
	if transport.MaxIdleConns != 1024 || transport.MaxIdleConnsPerHost != 1024 || transport.MaxConnsPerHost != 1024 {
		t.Fatalf("idle=%d perHost=%d max=%d, want 1024/1024/1024", transport.MaxIdleConns, transport.MaxIdleConnsPerHost, transport.MaxConnsPerHost)
	}
}

func TestSavedCookiesAvoidAnotherLogin(t *testing.T) {
	fixture := newRTAFixture(t)
	cookieFile := filepath.Join(t.TempDir(), "rta.cookies.json")
	first, _, _ := fixture.client(t, cookieFile)
	query := SalesQuery{
		BusinessStoreID: "STOREA",
		StartDate:       time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC),
		EndDate:         time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC),
	}
	if _, err := first.Sales(context.Background(), query); err != nil {
		t.Fatal(err)
	}
	loginCount := fixture.loginSubmissions.Load()
	second, _, _ := fixture.client(t, cookieFile)
	if _, err := second.Sales(context.Background(), query); err != nil {
		t.Fatal(err)
	}
	if fixture.loginSubmissions.Load() != loginCount {
		t.Fatalf("saved cookie was ignored; login submissions changed from %d to %d", loginCount, fixture.loginSubmissions.Load())
	}
}

func TestExpiredSalesSessionAutomaticallyRelogs(t *testing.T) {
	fixture := newRTAFixture(t)
	client, _, _ := fixture.client(t, "")
	query := SalesQuery{
		BusinessStoreID: "STOREA",
		StartDate:       time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC),
		EndDate:         time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC),
	}
	if _, err := client.Sales(context.Background(), query); err != nil {
		t.Fatal(err)
	}
	fixture.mu.Lock()
	fixture.expireNextSales = true
	fixture.mu.Unlock()
	before := fixture.loginSubmissions.Load()
	_, err := client.Sales(context.Background(), query)
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

func TestSalesSplitsRangesWiderThan90DaysAndMergesItems(t *testing.T) {
	fixture := newRTAFixture(t)
	client, _, _ := fixture.client(t, "")
	result, err := client.Sales(context.Background(), SalesQuery{
		BusinessStoreID: "STOREA",
		StartDate:       time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
		EndDate:         time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StartDate != "2026-04-02" || result.EndDate != "2026-07-01" {
		t.Fatalf("merged result dates=%s to %s, want original range", result.StartDate, result.EndDate)
	}
	if len(result.Items) != 2 || result.Items[0].Matnr != "SKU-1" || result.Items[1].Matnr != "SKU-2" {
		t.Fatalf("window items were not merged: %+v", result.Items)
	}
	if result.Items[0].TPSaleAmount != 21 || result.Items[1].TPSaleAmount != 41 {
		t.Fatalf("merged item amounts=%v/%v, want 21/41", result.Items[0].TPSaleAmount, result.Items[1].TPSaleAmount)
	}
	if result.TotalAmount != 62 || result.GrossQuantity != 6 {
		t.Fatalf("merged aggregates total=%v quantity=%v, want 62/6", result.TotalAmount, result.GrossQuantity)
	}
	if result.TrendGrossSaleAmount == nil || *result.TrendGrossSaleAmount != 5300.75 {
		t.Fatalf("merged Trend View sales=%v, want 5300.75 from the window that contains June fixture rows", result.TrendGrossSaleAmount)
	}
	if result.TotalTransactionCount == nil || *result.TotalTransactionCount != 4431 {
		t.Fatalf("merged Trend View transactions=%v, want 4431", result.TotalTransactionCount)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.payloads) != 4 {
		t.Fatalf("article payloads=%d, want 4 (2 windows x 2 pages)", len(fixture.payloads))
	}
	gotWindows := make([]string, 0, 2)
	seen := make(map[string]struct{})
	for _, payload := range fixture.payloads {
		key := payload.CurrentStartDay + "~" + payload.CurrentEndDay
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		gotWindows = append(gotWindows, key)
	}
	if len(gotWindows) != 2 || gotWindows[0] != "20260402~20260630" || gotWindows[1] != "20260701~20260701" {
		t.Fatalf("article windows=%v, want 20260402~20260630 then 20260701~20260701", gotWindows)
	}
	if len(fixture.transactionPayloads) != 2 {
		t.Fatalf("Trend View payloads=%d, want 2", len(fixture.transactionPayloads))
	}
	if fixture.transactionPayloads[0].CurrentStart != "2026-03-23" || fixture.transactionPayloads[0].CurrentEnd != "2026-06-20" {
		t.Fatalf("first Trend View window=%+v", fixture.transactionPayloads[0])
	}
	if fixture.transactionPayloads[1].CurrentStart != "2026-06-21" || fixture.transactionPayloads[1].CurrentEnd != "2026-07-01" {
		t.Fatalf("second Trend View window=%+v", fixture.transactionPayloads[1])
	}
	if len(result.TrendDays) != 3 {
		t.Fatalf("merged Trend days=%d, want 25-27 June", len(result.TrendDays))
	}
}

func TestSalesSplitsWideRangeWithoutTrendWhenSkipped(t *testing.T) {
	fixture := newRTAFixture(t)
	client, _, _ := fixture.client(t, "")
	result, err := client.Sales(context.Background(), SalesQuery{
		BusinessStoreID: "STOREA",
		StartDate:       time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
		EndDate:         time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		SkipTrend:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if fixture.transactionRequests.Load() != 0 {
		t.Fatalf("Trend View requests=%d, want 0", fixture.transactionRequests.Load())
	}
	if result.TrendGrossSaleAmount != nil || result.TotalTransactionCount != nil {
		t.Fatalf("skipped Trend View returned totals: amount=%v transactions=%v", result.TrendGrossSaleAmount, result.TotalTransactionCount)
	}
	if len(result.Items) != 2 || result.TotalAmount != 62 {
		t.Fatalf("skipped-trend merge failed: items=%d total=%v", len(result.Items), result.TotalAmount)
	}
}

func TestSalesKeepsA90DayRangeAsOneRequest(t *testing.T) {
	fixture := newRTAFixture(t)
	client, _, _ := fixture.client(t, "")
	_, err := client.Sales(context.Background(), SalesQuery{
		BusinessStoreID: "STOREA",
		StartDate:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:         time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.payloads) != 2 {
		t.Fatalf("article payloads=%d, want 2 pages of one window", len(fixture.payloads))
	}
	for _, payload := range fixture.payloads {
		if payload.CurrentStartDay != "20260101" || payload.CurrentEndDay != "20260331" {
			t.Fatalf("90-day range was split: %+v", payload)
		}
	}
	if len(fixture.transactionPayloads) != 2 {
		t.Fatalf("Trend View payloads=%d, want 2 after weekly lookback", len(fixture.transactionPayloads))
	}
	if fixture.transactionPayloads[0].CurrentStart != "2025-12-22" || fixture.transactionPayloads[0].CurrentEnd != "2026-03-21" {
		t.Fatalf("first Trend View lookback window=%+v", fixture.transactionPayloads[0])
	}
	if fixture.transactionPayloads[1].CurrentStart != "2026-03-22" || fixture.transactionPayloads[1].CurrentEnd != "2026-03-31" {
		t.Fatalf("second Trend View window=%+v", fixture.transactionPayloads[1])
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

func TestStoresReturnsAuthorizedBusinessValues(t *testing.T) {
	fixture := newRTAFixture(t)
	client, _, _ := fixture.client(t, "")
	stores, err := client.Stores(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(stores) != 2 || stores[0].BusinessID != "STOREA" || stores[0].Label != "STOREA-Fixture Store A" || stores[1].BusinessID != "STOREB" || stores[1].Label != "STOREB-Fixture Store B" {
		t.Fatalf("unexpected authorized stores: %+v", stores)
	}
	if fixture.loginSubmissions.Load() != 2 || fixture.storeRequests.Load() != 2 || fixture.salesRequests.Load() != 0 {
		t.Fatalf("login=%d stores=%d sales=%d, want 2/2/0", fixture.loginSubmissions.Load(), fixture.storeRequests.Load(), fixture.salesRequests.Load())
	}
}

func TestAuthorizedStoreValuePreservesStringPrefix(t *testing.T) {
	businessID, label, ok := splitAuthorizedStoreValue(" 00A-Fixture Store - Branch ")
	if !ok || businessID != "00A" || label != "00A-Fixture Store - Branch" {
		t.Fatalf("businessID=%q label=%q ok=%t", businessID, label, ok)
	}
}

func TestStoresCachesAndRefreshStoresReloads(t *testing.T) {
	fixture := newRTAFixture(t)
	client, _, _ := fixture.client(t, "")
	if _, err := client.Stores(context.Background()); err != nil {
		t.Fatal(err)
	}
	afterFirstLoad := fixture.storeRequests.Load()
	if _, err := client.Stores(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fixture.storeRequests.Load() != afterFirstLoad {
		t.Fatal("Stores ignored the authorized-store cache")
	}
	if _, err := client.RefreshStores(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fixture.storeRequests.Load() != afterFirstLoad+1 {
		t.Fatal("RefreshStores did not reload the authorized-store list")
	}
}

func TestStoreLookupIsExact(t *testing.T) {
	fixture := newRTAFixture(t)
	client, _, _ := fixture.client(t, "")
	_, err := client.Sales(context.Background(), SalesQuery{
		BusinessStoreID: "STORE",
		StartDate:       time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC),
		EndDate:         time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC),
	})
	var missing *StoreNotFoundError
	if !errors.As(err, &missing) || missing.BusinessStoreID != "STORE" {
		t.Fatalf("unexpected error: %T %v", err, err)
	}
	if fixture.salesRequests.Load() != 0 {
		t.Fatal("unknown business store unexpectedly reached the sales endpoint")
	}
}

func TestConcurrentStoresSharesAuthorizedStoreLoad(t *testing.T) {
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
			if err == nil && len(stores) != 2 {
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
	if fixture.loginSubmissions.Load() != 2 || fixture.storeRequests.Load() != 2 || fixture.salesRequests.Load() != 0 {
		t.Fatalf("login=%d stores=%d sales=%d, want one shared authorized-store load", fixture.loginSubmissions.Load(), fixture.storeRequests.Load(), fixture.salesRequests.Load())
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

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func inclusiveDaysYYYYMMDD(start, end string) int {
	return inclusiveDaysBetween(start, end, "20060102")
}

func inclusiveDaysISODate(start, end string) int {
	return inclusiveDaysBetween(start, end, "2006-01-02")
}

func inclusiveDaysBetween(start, end, layout string) int {
	parsedStart, startErr := time.Parse(layout, start)
	parsedEnd, endErr := time.Parse(layout, end)
	if startErr != nil || endErr != nil || parsedEnd.Before(parsedStart) {
		return 0
	}
	return int(parsedEnd.Sub(parsedStart)/(24*time.Hour)) + 1
}
