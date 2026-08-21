package rtasales

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

func compactSalesQuery() SalesQuery {
	day := time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC)
	return SalesQuery{
		BusinessStoreID: "STOREA",
		StartDate:       day,
		EndDate:         day,
		SkipTrend:       true,
		Compact:         true,
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		value string
		want  time.Duration
	}{
		{name: "empty", value: "", want: 0},
		{name: "seconds", value: "2", want: 2 * time.Second},
		{name: "capped", value: "120", want: maxRetryAfter},
		{name: "zero", value: "0", want: 0},
		{name: "http date", value: now.Add(5 * time.Second).Format(http.TimeFormat), want: 5 * time.Second},
		{name: "past date", value: now.Add(-time.Second).Format(http.TimeFormat), want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := parseRetryAfter(test.value, now)
			if got != test.want {
				t.Fatalf("Retry-After %q = %s, want %s", test.value, got, test.want)
			}
		})
	}
}

func TestSalesRetries429ThenSucceeds(t *testing.T) {
	fixture := newRTAFixture(t)
	fixture.salesFailRemaining = 1
	fixture.salesFailStatus = http.StatusTooManyRequests
	fixture.salesRetryAfter = "2"
	client, _, _ := fixture.client(t, "")
	var delays []time.Duration
	client.retryWait = func(ctx context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return ctx.Err()
	}
	if _, err := client.Sales(context.Background(), compactSalesQuery()); err != nil {
		t.Fatal(err)
	}
	if len(delays) != 1 || delays[0] != 2*time.Second {
		t.Fatalf("delays=%v, want one 2s Retry-After wait", delays)
	}
}

func TestSalesRetries5xxThenSucceeds(t *testing.T) {
	fixture := newRTAFixture(t)
	fixture.salesFailRemaining = 2
	fixture.salesFailStatus = http.StatusBadGateway
	client, _, _ := fixture.client(t, "")
	var delays []time.Duration
	client.retryWait = func(ctx context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return ctx.Err()
	}
	if _, err := client.Sales(context.Background(), compactSalesQuery()); err != nil {
		t.Fatal(err)
	}
	if len(delays) != 2 || delays[0] != time.Second || delays[1] != 3*time.Second {
		t.Fatalf("delays=%v, want 1s then 3s", delays)
	}
}

func TestSalesRetriesTimeoutThenSucceeds(t *testing.T) {
	fixture := newRTAFixture(t)
	client, _, _ := fixture.client(t, "")
	transport := &failThenOKRoundTripper{
		remaining: 1,
		err:       &net.DNSError{Err: "i/o timeout", IsTimeout: true},
		next:      client.httpClient.Transport,
	}
	if transport.next == nil {
		transport.next = http.DefaultTransport
	}
	client.httpClient.Transport = transport
	var delays []time.Duration
	client.retryWait = func(ctx context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return ctx.Err()
	}
	if _, err := client.Sales(context.Background(), compactSalesQuery()); err != nil {
		t.Fatal(err)
	}
	if len(delays) == 0 {
		t.Fatal("timeout was not retried")
	}
}

func TestSalesDoesNotRetryHTTP403(t *testing.T) {
	fixture := newRTAFixture(t)
	fixture.salesFailRemaining = 8
	fixture.salesFailStatus = http.StatusForbidden
	client, _, _ := fixture.client(t, "")
	var waits int
	client.retryWait = func(ctx context.Context, _ time.Duration) error {
		waits++
		return ctx.Err()
	}
	_, err := client.Sales(context.Background(), compactSalesQuery())
	var auth *AuthError
	if !errors.As(err, &auth) {
		t.Fatalf("error=%T %v, want AuthError", err, err)
	}
	if waits != 0 {
		t.Fatalf("HTTP 403 was retried %d times", waits)
	}
}

func TestSalesDoesNotRetryHTTP401(t *testing.T) {
	fixture := newRTAFixture(t)
	fixture.salesFailRemaining = 8
	fixture.salesFailStatus = http.StatusUnauthorized
	client, _, _ := fixture.client(t, "")
	var waits int
	client.retryWait = func(context.Context, time.Duration) error {
		waits++
		return nil
	}
	_, err := client.Sales(context.Background(), compactSalesQuery())
	var auth *AuthError
	if !errors.As(err, &auth) {
		t.Fatalf("error=%T %v, want AuthError", err, err)
	}
	if waits != 0 {
		t.Fatalf("HTTP 401 was retried %d times", waits)
	}
}

func TestSalesTreatsPermissionEnvelopeAsAuthError(t *testing.T) {
	fixture := newRTAFixture(t)
	body, err := json.Marshal(map[string]any{"code": "1005", "msg": "没有权限访问该门店"})
	if err != nil {
		t.Fatal(err)
	}
	fixture.salesFailRemaining = 8
	fixture.salesFailStatus = http.StatusOK
	fixture.salesFailBody = body
	client, _, _ := fixture.client(t, "")
	var waits int
	client.retryWait = func(context.Context, time.Duration) error {
		waits++
		return nil
	}
	_, salesErr := client.Sales(context.Background(), compactSalesQuery())
	var auth *AuthError
	if !errors.As(salesErr, &auth) || auth.Code != "1005" {
		t.Fatalf("error=%T %v, want AuthError code 1005", salesErr, salesErr)
	}
	if waits != 0 {
		t.Fatalf("permission denial was retried %d times", waits)
	}
}

func TestSalesRetriesJSONRateLimitThenSucceeds(t *testing.T) {
	fixture := newRTAFixture(t)
	body, err := json.Marshal(map[string]any{"code": "429", "msg": "请求过于频繁"})
	if err != nil {
		t.Fatal(err)
	}
	fixture.salesFailRemaining = 1
	fixture.salesFailStatus = http.StatusOK
	fixture.salesFailBody = body
	client, _, _ := fixture.client(t, "")
	var delays []time.Duration
	client.retryWait = func(ctx context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return ctx.Err()
	}
	if _, err := client.Sales(context.Background(), compactSalesQuery()); err != nil {
		t.Fatal(err)
	}
	if len(delays) != 1 || delays[0] != time.Second {
		t.Fatalf("delays=%v, want the default 1s backoff", delays)
	}
}

func TestOneAccountSerializesStoresAndHonorsRetryAfter(t *testing.T) {
	fixture := newRTAFixture(t)
	fixture.salesHold = 25 * time.Millisecond
	fixture.salesFailRemaining = 1
	fixture.salesFailStatus = http.StatusTooManyRequests
	fixture.salesRetryAfter = "1"
	client, _, _ := fixture.client(t, "")
	var (
		mu     sync.Mutex
		delays []time.Duration
	)
	client.retryWait = func(ctx context.Context, delay time.Duration) error {
		mu.Lock()
		delays = append(delays, delay)
		mu.Unlock()
		return ctx.Err()
	}

	started := make(chan struct{}, 2)
	errorsFound := make(chan error, 2)
	for _, storeID := range []string{"STOREA", "STOREB"} {
		go func(storeID string) {
			started <- struct{}{}
			query := compactSalesQuery()
			query.BusinessStoreID = storeID
			_, err := client.Sales(context.Background(), query)
			errorsFound <- err
		}(storeID)
	}
	<-started
	<-started
	for range 2 {
		if err := <-errorsFound; err != nil {
			t.Fatal(err)
		}
	}
	fixture.mu.Lock()
	maxInFlight := fixture.salesMaxInFlight
	fixture.mu.Unlock()
	if maxInFlight != 1 {
		t.Fatalf("in-flight sales HTTP=%d, want 1 serialized account lane", maxInFlight)
	}
	mu.Lock()
	got := append([]time.Duration(nil), delays...)
	mu.Unlock()
	if len(got) != 1 || got[0] != time.Second {
		t.Fatalf("delays=%v, want one shared Retry-After wait", got)
	}
}

func TestSalesStopsRetryWhenContextCanceled(t *testing.T) {
	fixture := newRTAFixture(t)
	fixture.salesFailRemaining = 8
	fixture.salesFailStatus = http.StatusTooManyRequests
	client, _, _ := fixture.client(t, "")
	ctx, cancel := context.WithCancel(context.Background())
	client.retryWait = func(context.Context, time.Duration) error {
		cancel()
		return context.Canceled
	}
	_, err := client.Sales(ctx, compactSalesQuery())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v, want context canceled", err)
	}
}

func TestExhausted429RemainsUpstreamError(t *testing.T) {
	fixture := newRTAFixture(t)
	fixture.salesFailRemaining = 8
	fixture.salesFailStatus = http.StatusTooManyRequests
	client, _, _ := fixture.client(t, "")
	_, err := client.Sales(context.Background(), compactSalesQuery())
	var upstream *UpstreamError
	if !errors.As(err, &upstream) || upstream.StatusCode != http.StatusTooManyRequests || !upstream.Retryable() {
		t.Fatalf("error=%T %v, want retryable 429 UpstreamError", err, err)
	}
	var auth *AuthError
	if errors.As(err, &auth) {
		t.Fatal("429 was classified as an auth/permission error")
	}
}

func TestIsRetryableKeepsPermissionDistinct(t *testing.T) {
	if IsRetryable(&AuthError{Code: "403", Message: "denied"}) {
		t.Fatal("AuthError must not be retried")
	}
	if IsRetryable(&StoreNotFoundError{BusinessStoreID: "S04"}) {
		t.Fatal("missing store permission must not be retried")
	}
	if IsRetryable(&UpstreamError{Operation: "sales", StatusCode: 401}) || IsRetryable(&UpstreamError{Operation: "sales", StatusCode: 403}) {
		t.Fatal("HTTP 401/403 must not be retried")
	}
	if !IsRetryable(&UpstreamError{Operation: "sales", StatusCode: 429}) {
		t.Fatal("HTTP 429 should be retryable")
	}
	if !IsRetryable(&UpstreamError{Operation: "sales", Err: context.DeadlineExceeded}) {
		t.Fatal("transport timeout should be retryable")
	}
	if IsRetryable(&ProtocolError{Operation: "sales", Message: "RTA code 1005: 没有权限访问该门店"}) {
		t.Fatal("permission protocol errors must not be retried")
	}
}

type failThenOKRoundTripper struct {
	remaining int
	err       error
	next      http.RoundTripper
}

func (transport *failThenOKRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if transport.remaining > 0 {
		transport.remaining--
		return nil, transport.err
	}
	return transport.next.RoundTrip(request)
}
