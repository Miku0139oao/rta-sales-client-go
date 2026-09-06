// Package portableupdate implements the unprivileged, fixed-repository portable
// update protocol. Release discovery never downloads an executable.
package portableupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	Repository            = "Miku0139oao/rta-sales-client-go"
	ExecutableAsset       = "RTA-Excel-Filler-portable.exe"
	ChecksumsAsset        = "SHA256SUMS.txt"
	latestURL             = "https://api.github.com/repos/" + Repository + "/releases/latest"
	metadataLimit   int64 = 2 << 20
	checksumLimit   int64 = 64 << 10
	executableLimit int64 = 256 << 20
)

var stableVersion = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

type Version [3]uint64

func ParseVersion(text string) (Version, error) {
	var v Version
	parts := stableVersion.FindStringSubmatch(text)
	if parts == nil {
		return v, errors.New("unknown or non-stable version")
	}
	for i := range v {
		n, err := strconv.ParseUint(parts[i+1], 10, 32)
		if err != nil {
			return Version{}, errors.New("version component overflow")
		}
		v[i] = n
	}
	return v, nil
}

func (v Version) NewerThan(old Version) bool {
	for i := range v {
		if v[i] != old[i] {
			return v[i] > old[i]
		}
	}
	return false
}

// Candidate is backend-owned. URLs and sizes cannot be constructed by a caller
// outside this package and are never returned to the frontend.
type Candidate struct {
	version       string
	notes         string
	executableURL string
	checksumURL   string
	size          int64
}

func (c Candidate) Version() string { return c.version }
func (c Candidate) Notes() string   { return c.notes }

type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}
type release struct {
	Tag        string         `json:"tag_name"`
	Draft      bool           `json:"draft"`
	Prerelease bool           `json:"prerelease"`
	Body       string         `json:"body"`
	Assets     []releaseAsset `json:"assets"`
}

// Client has no UI-configurable endpoint, proxy, transport, or timeout.
// Tests in this package inject a transport without weakening production policy.
type Client struct{ http *http.Client }

func NewClient() *Client {
	transport := newUpdateTransport()
	return &Client{http: &http.Client{
		Transport: transport,
		Timeout:   3 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many update redirects")
			}
			return allowedURL(req.URL)
		},
	}}
}

func allowedURL(u *url.URL) error {
	if u.Scheme != "https" || u.User != nil || u.Fragment != "" || (u.Port() != "" && u.Port() != "443") {
		return errors.New("update URL must use HTTPS without credentials or fragment")
	}
	switch u.Hostname() {
	case "api.github.com", "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com":
		return nil
	default:
		return errors.New("update host is not allowed")
	}
}

func (c *Client) get(ctx context.Context, rawURL string, limit int64, out io.Writer) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if err := allowedURL(u); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "RTA-Excel-Filler-updater")
	req.Header.Set("Accept", "application/octet-stream")
	if u.Hostname() == "api.github.com" {
		req.Header.Set("Accept", "application/vnd.github+json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("update request returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > limit {
		return errors.New("update response exceeds size limit")
	}
	n, err := io.Copy(out, io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return err
	}
	if n > limit {
		return errors.New("update response exceeds size limit")
	}
	return nil
}

func (c *Client) Check(ctx context.Context, current string) (*Candidate, error) {
	old, err := ParseVersion(current)
	if err != nil {
		return nil, fmt.Errorf("current build cannot update: %w", err)
	}
	var body strings.Builder
	if err := c.get(ctx, latestURL, metadataLimit, &body); err != nil {
		return nil, err
	}
	return parseRelease([]byte(body.String()), old)
}

func parseRelease(data []byte, old Version) (*Candidate, error) {
	var r release
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("invalid release metadata: %w", err)
	}
	v, err := ParseVersion(r.Tag)
	if err != nil || r.Draft || r.Prerelease || !strings.HasPrefix(r.Tag, "v") {
		return nil, errors.New("release is not a stable version")
	}
	if !v.NewerThan(old) {
		return nil, nil
	}
	candidate := &Candidate{version: strings.TrimPrefix(r.Tag, "v"), notes: r.Body}
	counts := map[string]int{}
	for _, asset := range r.Assets {
		if asset.Name != ExecutableAsset && asset.Name != ChecksumsAsset {
			continue
		}
		counts[asset.Name]++
		// The metadata must point at the exact tag and repository. CDN locations
		// are only accepted as redirects from this authenticated GitHub URL.
		expected := "https://github.com/" + Repository + "/releases/download/" + r.Tag + "/" + asset.Name
		if asset.URL != expected {
			return nil, errors.New("release asset URL does not match repository/tag/name")
		}
		if asset.Size <= 0 {
			return nil, errors.New("invalid release asset size")
		}
		if asset.Name == ExecutableAsset {
			if asset.Size > executableLimit {
				return nil, errors.New("executable exceeds size limit")
			}
			candidate.executableURL, candidate.size = asset.URL, asset.Size
		} else {
			if asset.Size > checksumLimit {
				return nil, errors.New("checksums exceed size limit")
			}
			candidate.checksumURL = asset.URL
		}
	}
	if counts[ExecutableAsset] != 1 || counts[ChecksumsAsset] != 1 {
		return nil, errors.New("release requires exactly one portable executable and checksum asset")
	}
	return candidate, nil
}

func parseChecksum(text string) ([32]byte, error) {
	var sum [32]byte
	found := 0
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(strings.TrimSuffix(line, "\r"))
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 {
			return sum, errors.New("malformed checksum entry")
		}
		name := strings.TrimPrefix(fields[1], "*")
		if name != ExecutableAsset {
			continue
		}
		found++
		raw, err := hex.DecodeString(fields[0])
		if err != nil || len(raw) != len(sum) {
			return sum, errors.New("invalid SHA256 checksum")
		}
		copy(sum[:], raw)
	}
	if found != 1 {
		return sum, errors.New("checksum must contain exactly one portable entry")
	}
	return sum, nil
}

// Download writes only after a caller explicitly selects a backend candidate.
// The caller owns a private, newly-created destination and MUST discard it on
// any error. A successful hash check is not permission to execute: Authenticode,
// publisher, PE architecture/version and the helper protocol remain mandatory.
func (c *Client) Download(ctx context.Context, candidate Candidate, out io.Writer) ([32]byte, error) {
	var zero [32]byte
	if candidate.executableURL == "" || candidate.checksumURL == "" || candidate.size <= 0 {
		return zero, errors.New("no checked candidate")
	}
	var checksums strings.Builder
	if err := c.get(ctx, candidate.checksumURL, checksumLimit, &checksums); err != nil {
		return zero, err
	}
	expected, err := parseChecksum(checksums.String())
	if err != nil {
		return zero, err
	}
	hash := sha256.New()
	counter := &countWriter{out: io.MultiWriter(out, hash)}
	if err := c.get(ctx, candidate.executableURL, candidate.size, counter); err != nil {
		return zero, err
	}
	if counter.n != candidate.size {
		return zero, errors.New("download size does not match release metadata")
	}
	var actual [32]byte
	copy(actual[:], hash.Sum(nil))
	if actual != expected {
		return zero, errors.New("portable executable SHA256 mismatch")
	}
	return actual, nil
}

type countWriter struct {
	out io.Writer
	n   int64
}

func (w *countWriter) Write(p []byte) (int, error) {
	n, err := w.out.Write(p)
	w.n += int64(n)
	return n, err
}
