package rtasales

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCIPinsPatchedGoInsteadOfGoModPatchZero(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "..", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	mod, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "..", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mod), "go 1.25.13") {
		t.Fatal("go.mod must require Go 1.25.13 or newer so stdlib CVEs after 1.25.0 stay closed")
	}
	if strings.Contains(text, "go-version-file:") {
		t.Fatal("ci.yml must pin a patched toolchain instead of trusting go-version-file")
	}
	if !strings.Contains(text, `GO_VERSION: "1.25.13"`) {
		t.Fatal("ci.yml must pin GO_VERSION to 1.25.13 or newer so stdlib CVEs are closed")
	}
	if strings.Contains(text, "pull_request") && !strings.Contains(text, "github.event_name != 'pull_request'") {
		t.Fatal("ci.yml must keep Windows build and vuln scan off pull requests")
	}
}
