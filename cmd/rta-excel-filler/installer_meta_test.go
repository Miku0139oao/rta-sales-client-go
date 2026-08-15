package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestInstallerProductMetadataMatchesWailsJSON(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	appDir := filepath.Dir(thisFile)
	wailsPath := filepath.Join(appDir, "wails.json")
	nshPath := filepath.Join(appDir, "build", "windows", "installer", "wails_tools.nsh")
	scriptPath := filepath.Join(appDir, "..", "..", "scripts", "build-desktop.ps1")

	raw, err := os.ReadFile(wailsPath)
	if err != nil {
		t.Fatal(err)
	}
	var info struct {
		Info struct {
			ProductName    string `json:"productName"`
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatal(err)
	}
	if info.Info.ProductName != "RTA 銷售分析" {
		t.Fatalf("wails.json productName = %q, want RTA 銷售分析", info.Info.ProductName)
	}
	if info.Info.ProductVersion == "" || info.Info.ProductVersion == "0.1.0" {
		t.Fatalf("wails.json productVersion = %q, want a shipped 0.2.x+ version", info.Info.ProductVersion)
	}

	nsh, err := os.ReadFile(nshPath)
	if err != nil {
		t.Fatal(err)
	}
	name := nshDefine(t, string(nsh), "INFO_PRODUCTNAME")
	version := nshDefine(t, string(nsh), "INFO_PRODUCTVERSION")
	if name != info.Info.ProductName {
		t.Fatalf("wails_tools.nsh INFO_PRODUCTNAME = %q, want %q from wails.json (Start menu uses this)", name, info.Info.ProductName)
	}
	if version != info.Info.ProductVersion {
		t.Fatalf("wails_tools.nsh INFO_PRODUCTVERSION = %q, want %q from wails.json", version, info.Info.ProductVersion)
	}

	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{`-DINFO_PRODUCTNAME=`, `-DINFO_PRODUCTVERSION=`} {
		if !strings.Contains(string(script), flag) {
			t.Fatalf("build-desktop.ps1 must pass %s so a regenerated nsh cannot ship stale defaults", flag)
		}
	}
}

func nshDefine(t *testing.T, source, key string) string {
	t.Helper()
	pattern := regexp.MustCompile(`!define\s+` + regexp.QuoteMeta(key) + `\s+"([^"]+)"`)
	match := pattern.FindStringSubmatch(source)
	if len(match) != 2 {
		t.Fatalf("missing !define %s in wails_tools.nsh", key)
	}
	return match[1]
}
