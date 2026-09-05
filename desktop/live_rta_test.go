//go:build live

package desktop

import (
	"bufio"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	rtasales "github.com/Miku0139oao/rta-sales-client-go/rtasales"
	"github.com/Miku0139oao/rta-sales-client-go/securestore"
)

func TestLiveOneAccountListsStoresAndQueriesOneStore(t *testing.T) {
	account, password, cookieFile := liveRTACredentials(t)
	client, err := rtasales.NewClient(rtasales.Config{
		Account:        account,
		Password:       password,
		CookieFile:     cookieFile,
		CaptchaSolvers: []rtasales.CaptchaSolver{rtasales.NewEmbeddedOCRSolver(rtasales.EmbeddedOCRConfig{})},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	stores, err := client.Stores(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(stores) == 0 {
		t.Fatal("authorized store list was empty")
	}
	t.Logf("authorized stores=%d first=%s %s", len(stores), stores[0].BusinessID, stores[0].Label)
	day := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	result, err := client.Sales(ctx, rtasales.SalesQuery{
		BusinessStoreID: stores[0].BusinessID,
		StartDate:       day,
		EndDate:         day,
		SkipTrend:       true,
		Compact:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("sales result was nil")
	}
	t.Logf("sales items=%d amount=%.2f", len(result.Items), result.TotalAmount)
}

func TestLiveSixteenStoreSimulationHitsOneAccount(t *testing.T) {
	account, password, cookieFile := liveRTACredentials(t)
	inner, err := rtasales.NewClient(rtasales.Config{
		Account:        account,
		Password:       password,
		CookieFile:     cookieFile,
		CaptchaSolvers: []rtasales.CaptchaSolver{rtasales.NewEmbeddedOCRSolver(rtasales.EmbeddedOCRConfig{})},
	})
	if err != nil {
		t.Fatal(err)
	}
	client := maybeSimulateClient(inner, 16)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	stores, err := client.Stores(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(stores) != 16 {
		t.Fatalf("simulated stores=%d, want 16", len(stores))
	}
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	type outcome struct {
		store string
		err   error
	}
	started := time.Now()
	results := make(chan outcome, len(stores))
	for _, store := range stores {
		go func(store rtasales.Store) {
			_, queryErr := client.Sales(ctx, rtasales.SalesQuery{
				BusinessStoreID: store.BusinessID,
				StartDate:       from,
				EndDate:         to,
				SkipTrend:       false,
				Compact:         true,
			})
			results <- outcome{store: store.BusinessID, err: queryErr}
		}(store)
	}
	ok, limited := 0, 0
	for range stores {
		item := <-results
		if item.err == nil {
			ok++
			continue
		}
		if strings.Contains(item.err.Error(), "429") {
			limited++
			t.Logf("store %s rate-limited: %v", item.store, item.err)
			continue
		}
		t.Fatalf("store %s failed: %v", item.store, item.err)
	}
	elapsed := time.Since(started)
	t.Logf("16-store one-period simulation took %s success=%d rateLimited=%d", elapsed.Round(time.Millisecond), ok, limited)
	if ok == 0 {
		t.Fatal("every simulated store failed")
	}
}

func TestLiveSixteenStoreOnePeriodConcurrencySweep(t *testing.T) {
	account, password, cookieFile := liveRTACredentials(t)
	inner, err := rtasales.NewClient(rtasales.Config{
		Account:        account,
		Password:       password,
		CookieFile:     cookieFile,
		CaptchaSolvers: []rtasales.CaptchaSolver{rtasales.NewEmbeddedOCRSolver(rtasales.EmbeddedOCRConfig{})},
	})
	if err != nil {
		t.Fatal(err)
	}
	client := maybeSimulateClient(inner, 16)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	stores, err := client.Stores(ctx)
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	type outcome struct{ err error }
	run := func(workers int) (time.Duration, int) {
		jobs := make(chan rtasales.Store)
		results := make(chan outcome, len(stores))
		started := time.Now()
		for range workers {
			go func() {
				for store := range jobs {
					_, queryErr := client.Sales(ctx, rtasales.SalesQuery{
						BusinessStoreID: store.BusinessID,
						StartDate:       from,
						EndDate:         to,
						SkipTrend:       true,
						Compact:         true,
					})
					results <- outcome{err: queryErr}
				}
			}()
		}
		for _, store := range stores {
			jobs <- store
		}
		close(jobs)
		ok := 0
		for range stores {
			if item := <-results; item.err == nil {
				ok++
			}
		}
		return time.Since(started), ok
	}
	for _, workers := range []int{16, 32, 80, 160} {
		elapsed, ok := run(workers)
		t.Logf("workers=%d took %s success=%d/%d", workers, elapsed.Round(time.Millisecond), ok, len(stores))
	}
}

func TestLiveSixteenStoreFivePeriodSimulation(t *testing.T) {
	account, password, cookieFile := liveRTACredentials(t)
	inner, err := rtasales.NewClient(rtasales.Config{
		Account:        account,
		Password:       password,
		CookieFile:     cookieFile,
		CaptchaSolvers: []rtasales.CaptchaSolver{rtasales.NewEmbeddedOCRSolver(rtasales.EmbeddedOCRConfig{})},
	})
	if err != nil {
		t.Fatal(err)
	}
	client := maybeSimulateClient(inner, 16)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	stores, err := client.Stores(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(stores) != 16 {
		t.Fatalf("simulated stores=%d, want 16", len(stores))
	}
	periods := []struct {
		from, to time.Time
		trend    bool
	}{
		{time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC), true},
		{time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC), true},
		{time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC), true},
		{time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 8, 16, 0, 0, 0, 0, time.UTC), true},
		{time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 9, 30, 0, 0, 0, 0, time.UTC), false},
	}
	type job struct {
		store  string
		period int
	}
	jobs := make([]job, 0, len(stores)*len(periods))
	for index := range periods {
		for _, store := range stores {
			jobs = append(jobs, job{store: store.BusinessID, period: index})
		}
	}
	type outcome struct {
		err error
	}
	started := time.Now()
	work := make(chan job)
	results := make(chan outcome, len(jobs))
	workers := 160
	for range workers {
		go func() {
			for item := range work {
				period := periods[item.period]
				_, queryErr := client.Sales(ctx, rtasales.SalesQuery{
					BusinessStoreID:   item.store,
					StartDate:         period.from,
					EndDate:           period.to,
					SkipTrend:         !period.trend,
					SkipTrendLookback: item.period != 0,
					Compact:           true,
				})
				results <- outcome{err: queryErr}
			}
		}()
	}
	for _, item := range jobs {
		work <- item
	}
	close(work)
	ok, limited, other := 0, 0, 0
	for range jobs {
		item := <-results
		switch {
		case item.err == nil:
			ok++
		case strings.Contains(item.err.Error(), "429"):
			limited++
		default:
			other++
			t.Logf("non-429 failure: %v", item.err)
		}
	}
	elapsed := time.Since(started)
	t.Logf("16-store 5-period simulation took %s jobs=%d success=%d rateLimited=%d other=%d", elapsed.Round(time.Millisecond), len(jobs), ok, limited, other)
	if ok == 0 {
		t.Fatal("every simulated query failed")
	}
}

func TestLiveTwoSessionsSameAccount(t *testing.T) {
	account, password, _ := liveRTACredentials(t)
	primaryHits := newLoginHitCounter()
	extraHits := newLoginHitCounter()
	primary, err := rtasales.NewClient(rtasales.Config{
		Account:        account,
		Password:       password,
		CookieStore:    new(securestore.MemoryCookieStore),
		CaptchaSolvers: []rtasales.CaptchaSolver{rtasales.NewEmbeddedOCRSolver(rtasales.EmbeddedOCRConfig{})},
		HTTPClient:     &http.Client{Timeout: 60 * time.Second, Transport: primaryHits},
	})
	if err != nil {
		t.Fatal(err)
	}
	extra, err := rtasales.NewClient(rtasales.Config{
		Account:        account,
		Password:       password,
		CookieStore:    new(securestore.MemoryCookieStore),
		CaptchaSolvers: []rtasales.CaptchaSolver{rtasales.NewEmbeddedOCRSolver(rtasales.EmbeddedOCRConfig{})},
		HTTPClient:     &http.Client{Timeout: 60 * time.Second, Transport: extraHits},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	stores, err := primary.Stores(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(stores) == 0 {
		t.Fatal("authorized store list was empty")
	}
	loginsAfterPrimary := primaryHits.logins.Load()
	t.Logf("primary login done stores=%d logins=%d captchas=%d", len(stores), loginsAfterPrimary, primaryHits.captchas.Load())

	extraStores, err := extra.Stores(ctx)
	if err != nil {
		t.Fatalf("second login failed: %v", err)
	}
	if len(extraStores) == 0 {
		t.Fatal("second session store list was empty")
	}
	t.Logf("extra login done stores=%d logins=%d captchas=%d", len(extraStores), extraHits.logins.Load(), extraHits.captchas.Load())

	refreshed, err := primary.RefreshStores(ctx)
	if err != nil {
		t.Fatalf("primary session failed after second login: %v", err)
	}
	if len(refreshed) == 0 {
		t.Fatal("primary store list was empty after second login")
	}
	loginsAfterSecond := primaryHits.logins.Load()
	kicked := loginsAfterSecond > loginsAfterPrimary
	t.Logf("primary after extra login stores=%d logins=%d kicked=%t", len(refreshed), loginsAfterSecond, kicked)

	day := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	query := rtasales.SalesQuery{
		BusinessStoreID: stores[0].BusinessID,
		StartDate:       day,
		EndDate:         day,
		SkipTrend:       true,
		Compact:         true,
	}
	type outcome struct {
		name string
		err  error
	}
	results := make(chan outcome, 2)
	go func() {
		_, queryErr := primary.Sales(ctx, query)
		results <- outcome{name: "primary", err: queryErr}
	}()
	go func() {
		_, queryErr := extra.Sales(ctx, query)
		results <- outcome{name: "extra", err: queryErr}
	}()
	for range 2 {
		item := <-results
		if item.err != nil {
			t.Fatalf("%s concurrent sales failed: %v", item.name, item.err)
		}
		t.Logf("%s concurrent sales ok", item.name)
	}

	if kicked {
		t.Fatal("RTA invalidated the first session after the second login")
	}
}

func TestLiveSixteenStoreExtraLogins(t *testing.T) {
	account, password, _ := liveRTACredentials(t)
	ctx, cancel := context.WithTimeout(context.Background(), 14*time.Minute)
	defer cancel()

	const sessionCount = 16
	hits := make([]*loginHitCounter, sessionCount)
	clients := make([]*rtasales.Client, sessionCount)
	for index := range sessionCount {
		hits[index] = newLoginHitCounter()
		client, err := newLiveMemoryClient(account, password, hits[index])
		if err != nil {
			t.Fatal(err)
		}
		clients[index] = client
	}

	stores, err := clients[0].Stores(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(stores) == 0 {
		t.Fatal("authorized store list was empty")
	}
	primaryLogins := hits[0].logins.Load()
	t.Logf("primary login done stores=%d logins=%d captchas=%d", len(stores), primaryLogins, hits[0].captchas.Load())

	for index := 1; index < sessionCount; index++ {
		extraStores, loginErr := clients[index].Stores(ctx)
		if loginErr != nil {
			t.Fatalf("session %d login failed: %v", index+1, loginErr)
		}
		if len(extraStores) == 0 {
			t.Fatalf("session %d store list was empty", index+1)
		}
		t.Logf("session %d login done stores=%d logins=%d captchas=%d", index+1, len(extraStores), hits[index].logins.Load(), hits[index].captchas.Load())
	}

	if _, err := clients[0].RefreshStores(ctx); err != nil {
		t.Fatalf("primary session failed after %d logins: %v", sessionCount, err)
	}
	kicked := hits[0].logins.Load() > primaryLogins
	t.Logf("primary after %d logins kicked=%t logins=%d", sessionCount, kicked, hits[0].logins.Load())

	simulated := expandSimulatedStores(stores, 16)
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	periods := []struct {
		from, to time.Time
		trend    bool
	}{
		{from, to, true},
		{time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC), true},
		{time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC), true},
		{time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 8, 16, 0, 0, 0, 0, time.UTC), true},
		{time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 9, 30, 0, 0, 0, 0, time.UTC), false},
	}
	before429 := loginHitTotal429(hits)
	started := time.Now()
	ok, limited, other := runSimulatedPeriodJobsAcrossSessions(ctx, clients, simulated, periods)
	elapsed := time.Since(started)
	http429 := loginHitTotal429(hits) - before429
	t.Logf("five-period sessions=%d took %s success=%d/%d rateLimited=%d other=%d http429=%d", sessionCount, elapsed.Round(time.Millisecond), ok, len(simulated)*len(periods), limited, other, http429)
	if ok == 0 {
		t.Fatal("every simulated query failed")
	}

	if kicked {
		t.Fatal("RTA invalidated the first session after extra logins")
	}
}

func newLiveMemoryClient(account, password string, transport http.RoundTripper) (*rtasales.Client, error) {
	return rtasales.NewClient(rtasales.Config{
		Account:        account,
		Password:       password,
		CookieStore:    new(securestore.MemoryCookieStore),
		CaptchaSolvers: []rtasales.CaptchaSolver{rtasales.NewEmbeddedOCRSolver(rtasales.EmbeddedOCRConfig{})},
		HTTPClient:     &http.Client{Timeout: 60 * time.Second, Transport: transport},
	})
}

type liveSessionJob struct {
	store rtasales.Store
	from  time.Time
	to    time.Time
	trend bool
}

func runSimulatedStoresAcrossSessions(ctx context.Context, clients []*rtasales.Client, stores []rtasales.Store, from, to time.Time, trend bool) (ok, limited, other int) {
	jobs := make([]liveSessionJob, 0, len(stores))
	for _, store := range stores {
		jobs = append(jobs, liveSessionJob{store: store, from: from, to: to, trend: trend})
	}
	return runSessionJobs(ctx, clients, jobs)
}

func runSimulatedPeriodJobsAcrossSessions(ctx context.Context, clients []*rtasales.Client, stores []rtasales.Store, periods []struct {
	from, to time.Time
	trend    bool
}) (ok, limited, other int) {
	jobs := make([]liveSessionJob, 0, len(stores)*len(periods))
	for _, period := range periods {
		for _, store := range stores {
			jobs = append(jobs, liveSessionJob{store: store, from: period.from, to: period.to, trend: period.trend})
		}
	}
	return runSessionJobs(ctx, clients, jobs)
}

func runSessionJobs(ctx context.Context, clients []*rtasales.Client, jobs []liveSessionJob) (ok, limited, other int) {
	type outcome struct{ err error }
	lanes := make([][]liveSessionJob, len(clients))
	for index, item := range jobs {
		lanes[index%len(clients)] = append(lanes[index%len(clients)], item)
	}
	results := make(chan outcome, len(jobs))
	var waitGroup sync.WaitGroup
	for index, lane := range lanes {
		if len(lane) == 0 {
			continue
		}
		waitGroup.Add(1)
		query := maybeSimulateClient(clients[index], 16)
		go func(query accountClient, lane []liveSessionJob) {
			defer waitGroup.Done()
			for _, item := range lane {
				_, queryErr := query.Sales(ctx, rtasales.SalesQuery{
					BusinessStoreID: item.store.BusinessID,
					StartDate:       item.from,
					EndDate:         item.to,
					SkipTrend:       !item.trend,
					Compact:         true,
				})
				results <- outcome{err: queryErr}
			}
		}(query, lane)
	}
	go func() {
		waitGroup.Wait()
		close(results)
	}()
	for item := range results {
		switch {
		case item.err == nil:
			ok++
		case strings.Contains(item.err.Error(), "429"):
			limited++
		default:
			other++
		}
	}
	return ok, limited, other
}

func loginHitTotal429(hits []*loginHitCounter) int64 {
	var total int64
	for _, item := range hits {
		total += item.status429.Load()
	}
	return total
}

type loginHitCounter struct {
	logins    atomic.Int64
	captchas  atomic.Int64
	status429 atomic.Int64
}

func newLoginHitCounter() *loginHitCounter {
	return &loginHitCounter{}
}

func (c *loginHitCounter) RoundTrip(request *http.Request) (*http.Response, error) {
	switch {
	case strings.Contains(request.URL.Path, "doLogin"):
		c.logins.Add(1)
	case strings.Contains(request.URL.Path, "getVerifyCodeImg"):
		c.captchas.Add(1)
	}
	response, err := http.DefaultTransport.RoundTrip(request)
	if err == nil && response != nil && response.StatusCode == http.StatusTooManyRequests {
		c.status429.Add(1)
	}
	return response, err
}

func liveRTACredentials(t *testing.T) (account, password, cookieFile string) {
	t.Helper()
	loadLiveDotEnv(t)
	account = strings.TrimSpace(os.Getenv("RTA_ACCOUNT"))
	password = strings.TrimSpace(os.Getenv("RTA_PASSWORD"))
	if account == "" || password == "" {
		t.Fatal("RTA_ACCOUNT and RTA_PASSWORD are required for live tests")
	}
	cookieFile = strings.TrimSpace(os.Getenv("RTA_COOKIE_FILE"))
	if cookieFile == "" {
		cookieFile = filepath.Join("..", ".rta-sales.cookies.json")
	}
	return account, password, cookieFile
}

func loadLiveDotEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{".env", filepath.Join("..", ".env")} {
		file, err := os.Open(name)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
				continue
			}
			key, value, _ := strings.Cut(line, "=")
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			value = strings.Trim(value, `"'`)
			if os.Getenv(key) == "" {
				t.Setenv(key, value)
			}
		}
		_ = file.Close()
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
		return
	}
}
