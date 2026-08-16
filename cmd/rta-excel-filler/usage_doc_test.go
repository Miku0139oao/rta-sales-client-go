package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUsageDocMatchesShippedUsageFlow(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	doc := filepath.Join(filepath.Dir(thisFile), "..", "..", "README.zh-TW.md")
	raw, err := os.ReadFile(doc)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, needle := range []string{
		"使用教學",
		"怎麼安裝",
		"第一次使用：先加帳號",
		"銷售分析",
		"匯出 PDF",
		"開啟資料夾",
		"把數字填進 Excel",
		"檔案名稱",
		"開啟檔案",
		"上移 / 下移",
		"RTA 銷售分析",
	} {
		if !strings.Contains(text, needle) {
			t.Errorf("README.zh-TW.md must mention %q", needle)
		}
	}
	for _, banned := range []string{"拖曳", "點點拖曳", "Drag the handle"} {
		if strings.Contains(text, banned) {
			t.Errorf("README.zh-TW.md still tells people to %q", banned)
		}
	}
}
