package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPortableProductMetadataMatchesWailsJSON(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test")
	}
	appDir := filepath.Dir(file)
	read := func(path string) []byte {
		t.Helper()
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	var config struct {
		Info struct {
			ProductName    string `json:"productName"`
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	if err := json.Unmarshal(read(filepath.Join(appDir, "wails.json")), &config); err != nil {
		t.Fatal(err)
	}
	var resource struct {
		Fixed struct {
			File    string `json:"file_version"`
			Product string `json:"product_version"`
		} `json:"fixed"`
		Info map[string]struct {
			Version string `json:"ProductVersion"`
			Name    string `json:"ProductName"`
		} `json:"info"`
	}
	if err := json.Unmarshal(read(filepath.Join(appDir, "build", "windows", "info.json")), &resource); err != nil {
		t.Fatal(err)
	}
	if config.Info.ProductName != "RTA 銷售分析" || config.Info.ProductVersion == "" {
		t.Fatal("invalid product metadata")
	}
	if resource.Fixed.File != config.Info.ProductVersion+".0" || resource.Fixed.Product != config.Info.ProductVersion+".0" || resource.Info["0409"].Version != config.Info.ProductVersion || resource.Info["0409"].Name != config.Info.ProductName {
		t.Fatal("resource/config versions differ")
	}
	script := string(read(filepath.Join(appDir, "..", "..", "scripts", "build-desktop.ps1")))
	for _, required := range []string{"sign-windows.ps1", "internal/buildinfo.Version=$Version", "Get-FileHash -Algorithm SHA256 -LiteralPath $portablePath"} {
		if !strings.Contains(script, required) {
			t.Errorf("missing %s", required)
		}
	}
	for _, banned := range []string{"makensis", "SkipInstaller", "SkipPortable", "-Filter '*.exe'", "RTA-Excel-Filler-setup.exe"} {
		if strings.Contains(script, banned) {
			t.Errorf("portable build contains %s", banned)
		}
	}
	publish := string(read(filepath.Join(appDir, "..", "..", "scripts", "publish-portable.ps1")))
	for _, required := range []string{"$info.ProductVersionRaw.ToString()", "$info.FileVersionRaw.ToString()", "Unsigned CI staging only", "--draft=false --latest"} {
		if !strings.Contains(publish, required) {
			t.Errorf("publisher missing safeguard %s", required)
		}
	}
	if strings.Contains(publish, "--notes ") || strings.Contains(publish, "--title ") {
		t.Error("publisher must preserve curated draft title and release notes")
	}
	ci := string(read(filepath.Join(appDir, "..", "..", ".github", "workflows", "ci.yml")))
	if !strings.Contains(ci, "$info.ProductVersionRaw.ToString()") {
		t.Error("CI must check numeric resource version independently of localized string lookup")
	}
	release := string(read(filepath.Join(appDir, "..", "..", ".github", "workflows", "release.yml")))
	if !strings.Contains(release, "--draft --verify-tag") || strings.Contains(release, "--clobber") || strings.Contains(release, "desktop-linux") || strings.Contains(release, "desktop-macos") {
		t.Fatal("CI must only stage new unsigned Windows drafts")
	}
}
