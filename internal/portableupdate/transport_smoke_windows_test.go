//go:build windows && native_smoke && portable_update_smoke

package portableupdate

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/Miku0139oao/rta-sales-client-go/internal/smokefixture"
)

func TestSmokeTransportExactURLsAndNonce(t *testing.T) {
	if NewPersistentClient().cache.path != "" {
		t.Fatal("smoke build accessed production metadata cache")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := smokefixture.Manifest{Root: root, Nonce: "0123456789abcdef0123456789abcdef", Port: 19437, Version: "0.4.7"}
	raw, _ := json.Marshal(m)
	for name, body := range map[string][]byte{"manifest.json": raw, "nonce": []byte(m.Nonce), "latest.json": []byte("fixture"), "next.exe": []byte("test-only-not-executable"), ChecksumsAsset: []byte("checksum")} {
		if err := os.WriteFile(filepath.Join(root, name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("RTA_PORTABLE_SMOKE_MANIFEST", filepath.Join(root, "manifest.json"))
	transport := newUpdateTransport()
	for _, url := range []string{latestURL, "https://github.com/" + Repository + "/releases/download/v0.4.7/" + ExecutableAsset, "https://github.com/" + Repository + "/releases/download/v0.4.7/" + ChecksumsAsset} {
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		resp, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || len(body) == 0 {
			t.Fatal("empty fixture")
		}
	}
	for _, url := range []string{"https://example.com/", "https://api.github.com/repos/" + Repository + "/releases/latest", latestURL + "?x=1", "https://github.com/other/repo/releases/latest"} {
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		if _, err := transport.RoundTrip(req); err == nil {
			t.Fatalf("accepted %s", url)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "nonce"), []byte("wrong"), 0600); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, latestURL, nil)
	if _, err := transport.RoundTrip(req); err == nil {
		t.Fatal("accepted wrong nonce")
	}
}
