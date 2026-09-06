package portableupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(body string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), ContentLength: -1, Header: http.Header{}}
}
func fixture() release {
	base := "https://github.com/" + Repository + "/releases/download/v0.5.0/"
	return release{Tag: "v0.5.0", Body: "<script>notes are text</script>", Assets: []releaseAsset{
		{Name: ExecutableAsset, URL: base + ExecutableAsset, Size: 4},
		{Name: ChecksumsAsset, URL: base + ChecksumsAsset, Size: 100},
	}}
}
func TestVersion(t *testing.T) {
	for _, text := range []string{"dev", "", "0.5", "1.2.3-beta", "1.2.3+build", "01.2.3", "1.2.-3", "4294967296.1.1", " 1.2.3"} {
		if _, err := ParseVersion(text); err == nil {
			t.Errorf("accepted %q", text)
		}
	}
	a, _ := ParseVersion("v0.10.0")
	b, _ := ParseVersion("0.9.9")
	if !a.NewerThan(b) || b.NewerThan(a) || a.NewerThan(a) {
		t.Fatal("non-numeric comparison")
	}
}
func TestReleaseValidation(t *testing.T) {
	old, _ := ParseVersion("0.4.5")
	cases := []struct {
		name   string
		mutate func(*release)
	}{
		{"draft", func(r *release) { r.Draft = true }},
		{"prerelease", func(r *release) { r.Prerelease = true }},
		{"unknown", func(r *release) { r.Tag = "dev" }},
		{"suffix", func(r *release) { r.Tag = "v0.5.0-beta" }},
		{"no prefix", func(r *release) { r.Tag = "0.5.0" }},
		{"duplicate", func(r *release) { r.Assets = append(r.Assets, r.Assets[0]) }},
		{"missing", func(r *release) { r.Assets = r.Assets[:1] }},
		{"wrong repo", func(r *release) { r.Assets[0].URL = strings.Replace(r.Assets[0].URL, Repository, "attacker/repo", 1) }},
		{"wrong size", func(r *release) { r.Assets[0].Size = 0 }},
		{"oversized", func(r *release) { r.Assets[0].Size = executableLimit + 1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := fixture()
			tc.mutate(&r)
			raw, _ := json.Marshal(r)
			if _, err := parseRelease(raw, old); err == nil {
				t.Fatal("accepted invalid metadata")
			}
		})
	}
	raw, _ := json.Marshal(fixture())
	c, err := parseRelease(raw, old)
	if err != nil || c.Version() != "0.5.0" || c.Notes() != fixture().Body {
		t.Fatalf("%+v %v", c, err)
	}
	for _, current := range []string{"0.5.0", "0.6.0"} {
		v, _ := ParseVersion(current)
		c, err := parseRelease(raw, v)
		if c != nil || err != nil {
			t.Fatal("offered downgrade or equal version")
		}
	}
	if _, err := parseRelease([]byte(`{} trailing`), old); err == nil {
		t.Fatal("accepted invalid JSON")
	}
}
func TestHostPolicy(t *testing.T) {
	for _, raw := range []string{"http://github.com/x", "https://github.com.evil.test/x", "https://github.com:444/x", "https://user@github.com/x", "https://github.com/x#fragment", "https://127.0.0.1/x", "file:///tmp/x"} {
		u, _ := url.Parse(raw)
		if allowedURL(u) == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	for _, host := range []string{"api.github.com", "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com"} {
		u, _ := url.Parse("https://" + host + "/x")
		if err := allowedURL(u); err != nil {
			t.Fatal(err)
		}
	}
}
func TestRedirects(t *testing.T) {
	for _, target := range []string{"http://github.com/x", "https://evil.test/x", "https://github.com/loop"} {
		t.Run(target, func(t *testing.T) {
			c := NewClient()
			calls := 0
			c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				resp := response("")
				resp.StatusCode = 302
				resp.Header.Set("Location", target)
				return resp, nil
			})
			if err := c.get(context.Background(), latestURL, 10, io.Discard); err == nil {
				t.Fatal("accepted redirect")
			}
			if calls > 5 {
				t.Fatalf("unbounded redirects: %d", calls)
			}
		})
	}
}
func TestCheckDoesNotDownload(t *testing.T) {
	c := NewClient()
	calls := 0
	raw, _ := json.Marshal(fixture())
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != latestURL {
			t.Fatal("download during check")
		}
		return response(string(raw)), nil
	})
	if _, err := c.Check(context.Background(), "dev"); err == nil || calls != 0 {
		t.Fatal("development build contacted release API")
	}
	if _, err := c.Check(context.Background(), "0.4.5"); err != nil || calls != 1 {
		t.Fatal(err, calls)
	}
}
func TestInspectLatestMetadataWithoutDownload(t *testing.T) {
	for _, current := range []string{"0.4.7", "0.5.0", "0.6.0"} {
		t.Run(current, func(t *testing.T) {
			c := NewClient()
			calls := 0
			raw, _ := json.Marshal(fixture())
			c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != latestURL {
					t.Fatal("artifact request during inspection")
				}
				return response(string(raw)), nil
			})
			result, err := c.Inspect(context.Background(), current)
			if err != nil || calls != 1 || result.Version != "0.5.0" || result.Body != fixture().Body {
				t.Fatalf("inspection=%+v calls=%d err=%v", result, calls, err)
			}
			if (result.Candidate != nil) != (current == "0.4.7") {
				t.Fatal("incorrect install eligibility")
			}
			if result.Candidate != nil && result.Candidate.Notes() != result.Body {
				t.Fatal("candidate notes mismatch")
			}
		})
	}
}

func TestInspectRejectsUnstableAndUnsafeCandidates(t *testing.T) {
	for _, mutate := range []func(*release){
		func(r *release) { r.Draft = true },
		func(r *release) { r.Prerelease = true },
		func(r *release) { r.Tag = "v0.5.0-beta" },
	} {
		for _, current := range []string{"0.4.7", "0.5.0", "0.6.0"} {
			r := fixture()
			mutate(&r)
			raw, _ := json.Marshal(r)
			old, _ := ParseVersion(current)
			result, err := inspectRelease(raw, old)
			if err == nil || result.Candidate != nil || result.Version != "" {
				t.Fatal("exposed unstable metadata", result, err)
			}
		}
	}
	for _, mutate := range []func(*release){
		func(r *release) { r.Assets = append(r.Assets, r.Assets[0]) },
		func(r *release) { r.Assets[0].URL = "https://github.com/attacker/repo/evil.exe" },
		func(r *release) { r.Assets[0].Size = 0 },
	} {
		r := fixture()
		mutate(&r)
		raw, _ := json.Marshal(r)
		for _, current := range []string{"0.4.7", "0.5.0", "0.6.0"} {
			old, _ := ParseVersion(current)
			result, err := inspectRelease(raw, old)
			if result.Candidate != nil {
				t.Fatal("unsafe installation candidate")
			}
			if current == "0.4.7" && err == nil {
				t.Fatal("unsafe newer assets accepted")
			}
		}
	}
}

func TestDownloadFailures(t *testing.T) {
	sum := sha256.Sum256([]byte("test"))
	valid := fmt.Sprintf("%x  %s\n", sum, ExecutableAsset)
	for _, tc := range []struct {
		name, checksums, exe string
		size                 int64
		wantErr              bool
	}{
		{"valid", valid, "test", 4, false},
		{"duplicate", valid + valid, "test", 4, true},
		{"binary duplicate", valid + fmt.Sprintf("%x *%s\n", sum, ExecutableAsset), "test", 4, true},
		{"missing", fmt.Sprintf("%x  other.exe\n", sum), "test", 4, true},
		{"invalid hash", "nope  " + ExecutableAsset, "test", 4, true},
		{"malformed", valid + "bad line here", "test", 4, true},
		{"tamper", valid, "evil", 4, true},
		{"truncated", valid, "tes", 4, true},
		{"oversized", valid, "test!", 4, true},
		{"checksum limit", strings.Repeat("x", int(checksumLimit+1)), "test", 4, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClient()
			exeCalls := 0
			c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if strings.HasSuffix(r.URL.Path, ChecksumsAsset) {
					return response(tc.checksums), nil
				}
				exeCalls++
				return response(tc.exe), nil
			})
			cand := Candidate{executableURL: "https://github.com/" + ExecutableAsset, checksumURL: "https://github.com/" + ChecksumsAsset, size: tc.size}
			var out bytes.Buffer
			_, err := c.Download(context.Background(), cand, &out)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error %v", err)
			}
			if _, checksumErr := parseChecksum(tc.checksums); checksumErr != nil && exeCalls != 0 {
				t.Fatal("download before checksum validation")
			}
		})
	}
}
func TestHTTPFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		length int64
		body   string
	}{
		{"http", 403, 0, ""}, {"declared limit", 200, 11, ""}, {"stream limit", 200, -1, "12345678901"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClient()
			c.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				r := response(tc.body)
				r.StatusCode = tc.status
				r.ContentLength = tc.length
				return r, nil
			})
			if c.get(context.Background(), latestURL, 10, io.Discard) == nil {
				t.Fatal("expected error")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := NewClient().get(ctx, latestURL, 10, io.Discard); err == nil {
		t.Fatal("ignored cancellation")
	}
}
