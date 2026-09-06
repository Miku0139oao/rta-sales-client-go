//go:build !portable_update_smoke

package portableupdate

import (
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func newUpdateCachePath() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "RTA-Excel-Filler", "updates-v1.json")
	}
	return ""
}

func newUpdateTransport() http.RoundTripper {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 15 * time.Second
	return transport
}
