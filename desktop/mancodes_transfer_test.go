package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestExportManCodeCatalogRoundTrip(t *testing.T) {
	app, root, _ := newTestApp(t, new(fakeEngine), fakeClients{})
	first, err := app.SaveManCodeGroup(SaveManCodeGroupRequest{Name: "保健", Raw: "456, 123，456\n789"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := app.SaveManCodeGroup(SaveManCodeGroupRequest{Name: "護膚", Codes: []string{"sku-b", "SKU-A"}})
	if err != nil {
		t.Fatal(err)
	}

	exportPath := filepath.Join(t.TempDir(), "item-codes.json")
	dialogs := app.dialogs.(*fakeDialogs)
	dialogs.save = exportPath

	exported, err := app.ExportManCodeCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if exported.Cancelled || exported.Path != exportPath {
		t.Fatalf("unexpected export result: %#v", exported)
	}
	if dialogs.lastSave.DefaultFilename != "item-codes.json" {
		t.Fatalf("unexpected save dialog defaults: %#v", dialogs.lastSave)
	}
	if len(dialogs.lastSave.Filters) != 1 || dialogs.lastSave.Filters[0].Pattern != "*.json" {
		t.Fatalf("export dialog filters = %#v", dialogs.lastSave.Filters)
	}

	data, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	var document manCodeDocument
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if document.Version != manCodeFileVersion || len(document.Groups) != 2 {
		t.Fatalf("exported document = %s", data)
	}
	if document.Groups[0].ID != first.ID || document.Groups[0].Name != "保健" {
		t.Fatalf("first exported group = %#v", document.Groups[0])
	}
	if !slices.Equal(document.Groups[0].Codes, []string{"123", "456", "789"}) {
		t.Fatalf("exported codes were not preserved: %#v", document.Groups[0].Codes)
	}
	if document.Groups[1].ID != second.ID || !slices.Equal(document.Groups[1].Codes, []string{"SKU-A", "sku-b"}) {
		t.Fatalf("second exported group = %#v", document.Groups[1])
	}

	if _, err := app.SaveManCodeGroup(SaveManCodeGroupRequest{Name: "個護", Codes: []string{"000"}}); err != nil {
		t.Fatal(err)
	}
	dialogs.open = exportPath
	imported, err := app.ImportManCodeCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if imported.Cancelled || len(imported.Groups) != 2 {
		t.Fatalf("unexpected import result: %#v", imported)
	}
	if len(dialogs.lastOpen.Filters) != 1 || dialogs.lastOpen.Filters[0].Pattern != "*.json" {
		t.Fatalf("import dialog filters = %#v", dialogs.lastOpen.Filters)
	}
	groups, err := app.ListManCodeGroups()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || groups[0].ID != first.ID || groups[1].ID != second.ID {
		t.Fatalf("round-trip catalog = %#v", groups)
	}
	if groups[0].Name != "保健" || !slices.Equal(groups[0].Codes, []string{"123", "456", "789"}) {
		t.Fatalf("round-trip first group = %#v", groups[0])
	}
	catalog, err := os.ReadFile(filepath.Join(root, "mancodes.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(catalog), first.ID) || strings.Contains(string(catalog), "個護") {
		t.Fatalf("live catalog was not replaced atomically: %s", catalog)
	}
}

func TestImportManCodeCatalogNormalizesCodesAndRejectsMalformed(t *testing.T) {
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{})
	if _, err := app.SaveManCodeGroup(SaveManCodeGroupRequest{Name: "保健", Codes: []string{"123"}}); err != nil {
		t.Fatal(err)
	}

	validID := "11111111-1111-4111-8111-111111111111"
	otherID := "22222222-2222-4222-8222-222222222222"
	source := filepath.Join(t.TempDir(), "incoming.json")
	if err := os.WriteFile(source, []byte(`{
  "version": 1,
  "groups": [
    {"id": "`+validID+`", "name": " 護膚 ", "codes": ["10, 2", "sku-a", "2"]}
  ]
}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	dialogs := app.dialogs.(*fakeDialogs)
	dialogs.open = source
	imported, err := app.ImportManCodeCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if imported.Cancelled || len(imported.Groups) != 1 || imported.Groups[0].ID != validID {
		t.Fatalf("normalized import = %#v", imported)
	}
	if imported.Groups[0].Name != "護膚" || !slices.Equal(imported.Groups[0].Codes, []string{"2", "10", "sku-a"}) {
		t.Fatalf("imported codes were not normalized: %#v", imported.Groups[0])
	}

	padded := filepath.Join(t.TempDir(), "padded.json")
	if err := os.WriteFile(padded, []byte(`{
  "version": 1,
  "groups": [
    {"id": "`+validID+`", "name": " 護膚 ", "codes": ["10, 2", "sku-a", "2"]}
  ]
}

	
`), 0o600); err != nil {
		t.Fatal(err)
	}
	dialogs.open = padded
	if _, err := app.ImportManCodeCatalog(); err != nil {
		t.Fatalf("trailing whitespace should be accepted: %v", err)
	}

	cases := []struct {
		name    string
		payload string
	}{
		{name: "not json", payload: "not-json"},
		{name: "unknown field", payload: `{"version":1,"groups":[],"extra":true}`},
		{name: "bad version", payload: `{"version":2,"groups":[]}`},
		{name: "invalid id", payload: `{"version":1,"groups":[{"id":"bad","name":"護膚","codes":[]}]}`},
		{name: "empty name", payload: `{"version":1,"groups":[{"id":"` + validID + `","name":"  ","codes":[]}]}`},
		{name: "duplicate id", payload: `{"version":1,"groups":[{"id":"` + validID + `","name":"A","codes":[]},{"id":"` + validID + `","name":"B","codes":[]}]}`},
		{name: "duplicate name", payload: `{"version":1,"groups":[{"id":"` + validID + `","name":"護膚","codes":[]},{"id":"` + otherID + `","name":"護膚","codes":[]}]}`},
		{name: "missing groups", payload: `{"version":1}`},
		{name: "null groups", payload: `{"version":1,"groups":null}`},
		{name: "trailing json", payload: `{"version":1,"groups":[{"id":"` + validID + `","name":"個護","codes":[]}]}{"version":1,"groups":[]}`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			broken := filepath.Join(t.TempDir(), "broken.json")
			if err := os.WriteFile(broken, []byte(testCase.payload), 0o600); err != nil {
				t.Fatal(err)
			}
			dialogs.open = broken
			if _, err := app.ImportManCodeCatalog(); err == nil {
				t.Fatal("expected malformed import to be rejected")
			}
			groups, err := app.ListManCodeGroups()
			if err != nil {
				t.Fatal(err)
			}
			if len(groups) != 1 || groups[0].ID != validID || groups[0].Name != "護膚" {
				t.Fatalf("malformed import changed catalog: %#v", groups)
			}
		})
	}
}

func TestImportManCodeCatalogMalformedLeavesExistingUnchanged(t *testing.T) {
	app, root, _ := newTestApp(t, new(fakeEngine), fakeClients{})
	created, err := app.SaveManCodeGroup(SaveManCodeGroupRequest{Name: "保健", Codes: []string{"123"}})
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(root, "mancodes.json"))
	if err != nil {
		t.Fatal(err)
	}

	validID := "11111111-1111-4111-8111-111111111111"
	cases := []struct {
		name    string
		payload string
	}{
		{name: "invalid id", payload: `{"version":1,"groups":[{"id":"bad","name":"護膚","codes":["1"]}]}`},
		{name: "missing groups", payload: `{"version":1}`},
		{name: "null groups", payload: `{"version":1,"groups":null}`},
		{name: "trailing json", payload: `{"version":1,"groups":[{"id":"` + validID + `","name":"護膚","codes":[]}]}{"version":1,"groups":[]}`},
	}
	dialogs := app.dialogs.(*fakeDialogs)
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			broken := filepath.Join(t.TempDir(), "broken.json")
			if err := os.WriteFile(broken, []byte(testCase.payload), 0o600); err != nil {
				t.Fatal(err)
			}
			dialogs.open = broken
			if _, err := app.ImportManCodeCatalog(); err == nil {
				t.Fatal("expected malformed import to be rejected")
			}
			after, err := os.ReadFile(filepath.Join(root, "mancodes.json"))
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatalf("malformed import rewrote catalog\nbefore: %s\nafter: %s", before, after)
			}
			groups, err := app.ListManCodeGroups()
			if err != nil || len(groups) != 1 || groups[0].ID != created.ID {
				t.Fatalf("existing catalog changed: %#v, %v", groups, err)
			}
		})
	}
}

func TestManCodeCatalogOversizedUpdateLeavesExistingUnchanged(t *testing.T) {
	app, root, _ := newTestApp(t, new(fakeEngine), fakeClients{})
	created, err := app.SaveManCodeGroup(SaveManCodeGroupRequest{Name: "保健", Codes: []string{"123"}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "mancodes.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := app.ReplaceManCodeGroupCodes(ReplaceManCodeGroupCodesRequest{
		ID: created.ID, Raw: strings.Repeat("X", maximumManCodeBytes),
	}); err == nil || !strings.Contains(err.Error(), "exceeds 1 MiB") {
		t.Fatalf("oversized update error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("oversized update rewrote the existing catalog")
	}
}

func TestManCodeCatalogTransferCancelIsNoOp(t *testing.T) {
	app, root, _ := newTestApp(t, new(fakeEngine), fakeClients{})
	created, err := app.SaveManCodeGroup(SaveManCodeGroupRequest{Name: "保健", Codes: []string{"123"}})
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(root, "mancodes.json"))
	if err != nil {
		t.Fatal(err)
	}

	dialogs := app.dialogs.(*fakeDialogs)
	dialogs.save = ""
	exported, err := app.ExportManCodeCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if !exported.Cancelled || exported.Path != "" {
		t.Fatalf("cancelled export = %#v", exported)
	}
	if matches, err := filepath.Glob(filepath.Join(root, "*.json")); err != nil || len(matches) != 1 {
		t.Fatalf("export cancel wrote extra files: %v %v", matches, err)
	}

	dialogs.open = ""
	imported, err := app.ImportManCodeCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if !imported.Cancelled || imported.Path != "" {
		t.Fatalf("cancelled import = %#v", imported)
	}
	after, err := os.ReadFile(filepath.Join(root, "mancodes.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("cancel rewrote catalog\nbefore: %s\nafter: %s", before, after)
	}
	groups, err := app.ListManCodeGroups()
	if err != nil || len(groups) != 1 || groups[0].ID != created.ID {
		t.Fatalf("cancel changed catalog: %#v, %v", groups, err)
	}
}
