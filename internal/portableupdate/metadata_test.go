package portableupdate

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPagesExactMetadataPolicy(t *testing.T) {
	c := NewClient()
	calls := 0
	raw, _ := json.Marshal(fixture())
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != latestURL || r.Method != "GET" || r.Header.Get("Accept") != "application/json" || r.Header.Get("Cache-Control") != "no-cache" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Fatal("unexpected metadata request", r)
		}
		return response(string(raw)), nil
	})
	if _, err := c.Inspect(context.Background(), "0.4.8"); err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"https://api.github.com/repos/" + Repository + "/releases/latest", latestURL + "?x=1", latestURL + "#x", strings.Replace(latestURL, "/updates/latest.json", "/other.json", 1)} {
		if err := c.get(context.Background(), u, metadataLimit, io.Discard); err == nil {
			t.Fatal("accepted", u)
		}
	}
	if calls != 1 {
		t.Fatal("policy issued forbidden requests", calls)
	}
	for _, target := range []string{latestURL, "https://github.com/x", "https://api.github.com/x", "https://release-assets.githubusercontent.com/x"} {
		calls = 0
		c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			resp := response("")
			resp.StatusCode = 302
			resp.Header.Set("Location", target)
			return resp, nil
		})
		if _, err := c.Inspect(context.Background(), "0.4.8"); err == nil || calls != 1 {
			t.Fatal("metadata redirect followed", calls, err)
		}
	}
}
func TestStartupCacheManualRefreshAndPersistence(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	calls := 0
	c := NewClient()
	c.cache.now = func() time.Time { return now }
	c.cache.path = filepath.Join(t.TempDir(), "updates.json")
	raw, _ := json.Marshal(fixture())
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != latestURL {
			t.Fatal("download during cache check")
		}
		return response(string(raw)), nil
	})
	c.http.Transport = transport
	check := func(startup bool, current string) Inspection {
		t.Helper()
		var r Inspection
		var err error
		if startup {
			r, err = c.InspectStartup(context.Background(), current)
		} else {
			r, err = c.Inspect(context.Background(), current)
		}
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	first := check(true, "0.4.8")
	second := check(true, "0.4.8")
	if calls != 1 || first.Candidate == second.Candidate {
		t.Fatal("cache reused candidate instead of validated metadata")
	}
	if r := check(true, "0.5.0"); r.Candidate != nil || r.Version != "0.5.0" {
		t.Fatal("cached current changelog lost", r)
	}
	check(false, "0.4.8")
	if calls != 2 {
		t.Fatal("manual did not revalidate")
	}
	path := c.cache.path
	c = NewClient()
	c.cache.path = path
	c.cache.now = func() time.Time { return now }
	c.http.Transport = transport
	check(true, "0.4.8")
	if calls != 2 {
		t.Fatal("persistent valid cache not loaded")
	}
	now = now.Add(time.Hour)
	check(true, "0.4.8")
	if calls != 3 {
		t.Fatal("TTL not bounded")
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "candidateId") || strings.Contains(string(data), "Authorization") {
		t.Fatal("persisted private state")
	}
}
func TestStartupFailureBackoffManualBypassAndCancellation(t *testing.T) {
	now := time.Now()
	c := NewClient()
	c.cache.now = func() time.Time { return now }
	calls := 0
	c.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) { calls++; return response(`{"tag_name":"v0.5.0"}`), nil })
	for i := 1; i <= 8; i++ {
		result, err := c.InspectStartup(context.Background(), "0.4.8")
		if err == nil || result.Candidate != nil {
			t.Fatal("invalid metadata candidate")
		}
		if c.cache.record.Retry.After(now.Add(time.Hour)) {
			t.Fatal("unbounded backoff")
		}
		count := calls
		_, _ = c.InspectStartup(context.Background(), "0.4.8")
		if calls != count {
			t.Fatal("startup hammered server")
		}
		now = c.cache.record.Retry
	}
	_, _ = c.Inspect(context.Background(), "0.4.8")
	if calls != 9 {
		t.Fatal("manual throttled locally", calls)
	}
	before := c.cache.record
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Inspect(ctx, "0.4.8"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if c.cache.record.Failures != before.Failures || !c.cache.record.Retry.Equal(before.Retry) || calls != 9 {
		t.Fatal("cancellation counted as failure")
	}
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) { cancel(); return nil, context.Canceled })
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	_, _ = c.Inspect(ctx, "0.4.8")
	if c.cache.record.Failures != before.Failures {
		t.Fatal("inflight cancellation counted")
	}
}
func TestMetadataTimeoutBacksOff(t *testing.T) {
	c := NewClient()
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := c.InspectStartup(ctx, "0.4.8"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if c.cache.record.Failures != 1 || !c.cache.record.Retry.After(time.Now()) {
		t.Fatal("timeout did not back off")
	}
}
func TestRateLimitAndRetryAfterBounds(t *testing.T) {
	now := time.Now()
	for _, value := range []string{"999999999999", "3601", now.Add(24 * time.Hour).UTC().Format(http.TimeFormat), "-1", "garbage", "0"} {
		d := retryDelay(value, now)
		if d < time.Second || d > time.Hour {
			t.Fatal(value, d)
		}
	}
	if retryDelay("120", now) != 2*time.Minute {
		t.Fatal("seconds not parsed")
	}
	if d := retryDelay(now.Add(10*time.Minute).UTC().Format(http.TimeFormat), now); d < 9*time.Minute || d > 10*time.Minute {
		t.Fatal("HTTP date not parsed", d)
	}
	c := NewClient()
	c.cache.now = func() time.Time { return now }
	c.cache.path = filepath.Join(t.TempDir(), "retry.json")
	calls := 0
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		r := response("")
		r.StatusCode = 429
		r.Header.Set("Retry-After", "120")
		return r, nil
	})
	c.http.Transport = transport
	if r, err := c.Inspect(context.Background(), "0.4.8"); err == nil || r.Candidate != nil || !strings.Contains(err.Error(), "try again after") {
		t.Fatal(r, err)
	}
	path := c.cache.path
	c = NewClient()
	c.cache.path = path
	c.cache.now = func() time.Time { return now }
	c.http.Transport = transport
	if _, err := c.Inspect(context.Background(), "0.4.8"); err == nil || !strings.Contains(err.Error(), "next check after") || calls != 1 {
		t.Fatal("manual bypassed server backoff", err, calls)
	}
	now = now.Add(2 * time.Minute)
	_, _ = c.Inspect(context.Background(), "0.4.8")
	if calls != 2 {
		t.Fatal("retry never expired")
	}
}
func TestPersistentCacheRejectsTamperAndBounds(t *testing.T) {
	now := time.Now()
	raw, _ := json.Marshal(fixture())
	good := cacheRecord{Metadata: raw, Fetched: now}
	for _, kind := range []string{"url", "digest", "flags", "future", "future retry", "failures", "oversize", "truncated", "stale"} {
		t.Run(kind, func(t *testing.T) {
			r := good
			switch kind {
			case "url":
				r.Metadata = []byte(strings.Replace(string(raw), Repository, "attacker/repo", 1))
			case "digest":
				r.Metadata = []byte(strings.Replace(string(raw), "sha256:", "sha512:", 1))
			case "flags":
				r.Metadata = []byte(strings.Replace(string(raw), `"draft":false`, `"draft":null`, 1))
			case "future":
				r.Fetched = now.Add(time.Second)
			case "future retry":
				r.Retry = now.Add(2 * time.Hour)
			case "failures":
				r.Failures = 99
			case "stale":
				r.Fetched = now.Add(-time.Hour)
			}
			data, _ := json.Marshal(r)
			if kind == "oversize" {
				data = []byte(strings.Repeat("x", int(metadataLimit)+4097))
			}
			if kind == "truncated" {
				data = []byte(`{"metadata":`)
			}
			c := NewClient()
			c.cache.path = filepath.Join(t.TempDir(), "cache.json")
			c.cache.now = func() time.Time { return now }
			if err := os.WriteFile(c.cache.path, data, 0600); err != nil {
				t.Fatal(err)
			}
			calls := 0
			c.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) { calls++; return response(string(raw)), nil })
			if _, err := c.InspectStartup(context.Background(), "0.4.8"); err != nil || calls != 1 {
				t.Fatal("untrusted cache accepted", err, calls)
			}
		})
	}
}
