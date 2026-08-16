//go:build live

package desktop

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	rtasales "github.com/Miku0139oao/rta-sales-client-go"
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
					BusinessStoreID: item.store,
					StartDate:       period.from,
					EndDate:         period.to,
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
