package portableupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const metadataTTL = time.Hour

type retryError struct {
	delay  time.Duration
	status int
}

func (e *retryError) Error() string {
	return fmt.Sprintf("update service is busy or access is limited (HTTP %d); try again after %s", e.status, e.delay.Round(time.Second))
}
func retryDelay(value string, now time.Time) time.Duration {
	d := time.Minute
	if n, err := strconv.ParseInt(value, 10, 64); err == nil {
		if n > 3600 {
			n = 3600
		}
		if n < 1 {
			n = 1
		}
		d = time.Duration(n) * time.Second
	} else if t, err := http.ParseTime(value); err == nil {
		d = t.Sub(now)
	}
	if d < time.Second {
		d = time.Second
	}
	if d > time.Hour {
		d = time.Hour
	}
	return d
}
func validDigest(s string) bool {
	if len(s) != 71 || !strings.HasPrefix(s, "sha256:") {
		return false
	}
	for _, c := range s[7:] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

type cacheRecord struct {
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	Fetched     time.Time       `json:"fetched"`
	Retry       time.Time       `json:"retry"`
	ServerRetry time.Time       `json:"serverRetry"`
	Failures    int             `json:"failures"`
}
type metadataCache struct {
	mu     sync.Mutex
	record cacheRecord
	path   string
	loaded bool
	now    func() time.Time
}

// NewPersistentClient stores only bounded public metadata and retry timestamps,
// never candidate IDs, credentials or executable data, in native application data.
func NewPersistentClient() *Client {
	c := NewClient()
	c.cache.path = newUpdateCachePath()
	return c
}
func (m *metadataCache) load(now time.Time) {
	if m.loaded {
		return
	}
	m.loaded = true
	if m.path == "" {
		return
	}
	f, err := os.Open(m.path)
	if err != nil {
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, metadataLimit+4097))
	if err != nil || len(data) > int(metadataLimit)+4096 {
		return
	}
	var r cacheRecord
	if json.Unmarshal(data, &r) != nil || r.Failures < 0 || r.Failures > 7 || r.Fetched.After(now) || r.Retry.After(now.Add(time.Hour)) || r.ServerRetry.After(now.Add(time.Hour)) {
		return
	}
	if len(r.Metadata) > 0 {
		if _, err := inspectRelease(r.Metadata, Version{}); err != nil {
			return
		}
		r.Metadata = publicMetadata(r.Metadata)
	}
	m.record = r
}

// Keep only the known public release fields, not arbitrary response extensions.
// Call only after complete inspectRelease validation.
func publicMetadata(data []byte) json.RawMessage {
	var r release
	if json.Unmarshal(data, &r) != nil {
		return nil
	}
	raw, err := json.Marshal(r)
	if err != nil || len(raw) > int(metadataLimit) {
		return nil
	}
	return raw
}
func (m *metadataCache) save() {
	if m.path == "" {
		return
	}
	data, err := json.Marshal(m.record)
	if err != nil || len(data) > int(metadataLimit)+4096 {
		return
	}
	if os.MkdirAll(filepath.Dir(m.path), 0700) != nil {
		return
	}
	f, err := os.CreateTemp(filepath.Dir(m.path), ".updates-*")
	if err != nil {
		return
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return
	}
	if f.Close() != nil {
		return
	}
	_ = os.Rename(name, m.path) // atomic replacement; failure never prevents checking
}

// InspectStartup uses a one-hour validated metadata cache and bounded failure
// backoff. Manual Inspect bypasses these local limits, but not server Retry-After.
func (c *Client) InspectStartup(ctx context.Context, current string) (Inspection, error) {
	return c.inspect(ctx, current, true)
}
func (c *Client) inspect(ctx context.Context, current string, startup bool) (Inspection, error) {
	old, err := ParseVersion(current)
	if err != nil {
		return Inspection{}, fmt.Errorf("current build cannot update: %w", err)
	}
	m := &c.cache
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Inspection{}, err
	}
	now := time.Now()
	if m.now != nil {
		now = m.now()
	}
	m.load(now)
	r := &m.record
	if startup && len(r.Metadata) > 0 && !r.Fetched.After(now) && now.Sub(r.Fetched) < metadataTTL {
		result, err := inspectRelease(r.Metadata, old)
		if err == nil {
			return result, nil
		}
		r.Metadata = nil
	}
	next := r.ServerRetry
	if startup && r.Retry.After(next) {
		next = r.Retry
	}
	if now.Before(next) {
		return Inspection{}, fmt.Errorf("update check deferred; next check after %s", next.Local().Format(time.RFC3339))
	}
	var body strings.Builder
	err = c.get(ctx, latestURL, metadataLimit, &body)
	var result Inspection
	if err == nil {
		result, err = inspectRelease([]byte(body.String()), old)
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return Inspection{}, ctx.Err()
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	} // timeouts are failures, explicit cancellation is not
	now = time.Now()
	if m.now != nil {
		now = m.now()
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return Inspection{}, err
		}
		r.Failures++
		if r.Failures > 7 {
			r.Failures = 7
		}
		delay := time.Minute * time.Duration(1<<uint(r.Failures-1))
		if delay > time.Hour {
			delay = time.Hour
		}
		r.Retry = now.Add(delay)
		var limited *retryError
		if errors.As(err, &limited) {
			r.ServerRetry = now.Add(limited.delay)
		}
		m.save()
		return Inspection{}, err
	}
	*r = cacheRecord{Metadata: publicMetadata([]byte(body.String())), Fetched: now}
	m.save()
	return result, nil
}
