//go:build !portable_update_smoke

package portableupdate

import (
	"net/http"
	"time"
)

func newUpdateTransport() http.RoundTripper {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 15 * time.Second
	return transport
}
