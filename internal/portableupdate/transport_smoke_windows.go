//go:build windows && native_smoke && portable_update_smoke

package portableupdate

import (
	"fmt"
	"github.com/Miku0139oao/rta-sales-client-go/internal/smokefixture"
	"net/http"
	"os"
)

type smokeTransport struct{}

func newUpdateTransport() http.RoundTripper { return smokeTransport{} }
func (smokeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	m, err := smokefixture.Load()
	if err != nil {
		return nil, err
	}
	if _, err = ParseVersion(m.Version); err != nil {
		return nil, err
	}
	base := "https://github.com/" + Repository + "/releases/download/v" + m.Version + "/"
	name := ""
	switch req.URL.String() {
	case latestURL:
		name = "latest.json"
	case base + ExecutableAsset:
		name = "next.exe"
	case base + ChecksumsAsset:
		name = ChecksumsAsset
	default:
		return nil, fmt.Errorf("smoke transport rejects URL %q", req.URL.String())
	}
	if req.Method != http.MethodGet {
		return nil, fmt.Errorf("smoke transport rejects method")
	}
	path, err := m.File(name)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	stat, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	return &http.Response{StatusCode: 200, Header: make(http.Header), ContentLength: stat.Size(), Body: f, Request: req}, nil
}
