package rtasales

import (
	"os"
	"strings"
	"testing"
)

func TestCIPinsPatchedGoInsteadOfGoModPatchZero(t *testing.T) {
	raw, err := os.ReadFile(".github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "go-version-file:") {
		t.Fatal("ci.yml must not use go-version-file: go.mod says 1.25.0 and that patch fails govulncheck")
	}
	if !strings.Contains(text, `GO_VERSION: "1.25.13"`) {
		t.Fatal("ci.yml must pin GO_VERSION to 1.25.13 or newer so stdlib CVEs are closed")
	}
	if strings.Contains(text, "pull_request") && !strings.Contains(text, "github.event_name != 'pull_request'") {
		t.Fatal("ci.yml must keep Windows build and vuln scan off pull requests")
	}
}
