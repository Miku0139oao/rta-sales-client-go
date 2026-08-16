package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestNormalizeManCodesSplitsDedupesAndSorts(t *testing.T) {
	got := normalizeManCodes("  10, 2，003\nabc  ABC, 2, foo  ", "SKU-B, sku-a")
	want := []string{"2", "003", "10", "ABC", "abc", "foo", "sku-a", "SKU-B"}
	if !slices.Equal(got, want) {
		t.Fatalf("normalizeManCodes = %#v, want %#v", got, want)
	}
	if got := normalizeManCodes("1,, ,2", "  ", ""); !slices.Equal(got, []string{"1", "2"}) {
		t.Fatalf("empty tokens were kept: %#v", got)
	}
	if got := normalizeManCodes(); len(got) != 0 {
		t.Fatalf("empty input = %#v, want empty slice", got)
	}
}

func TestManCodeCatalogMissingFileReturnsEmptyList(t *testing.T) {
	repository, err := NewFileManCodeRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	groups, err := repository.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("missing catalog = %#v, want empty list", groups)
	}
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{})
	listed, err := app.ListManCodeGroups()
	if err != nil || len(listed) != 0 {
		t.Fatalf("empty app catalog = %#v, %v", listed, err)
	}
}

func TestManCodeCatalogPersistsAndReloads(t *testing.T) {
	app, root, _ := newTestApp(t, new(fakeEngine), fakeClients{})
	created, err := app.SaveManCodeGroup(SaveManCodeGroupRequest{
		Name: " 保健 ",
		Raw:  "456, 123，456\n789",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "保健" || !validProfileID(created.ID) {
		t.Fatalf("unexpected created group: %#v", created)
	}
	if !slices.Equal(created.Codes, []string{"123", "456", "789"}) {
		t.Fatalf("codes were not normalized: %#v", created.Codes)
	}
	data, err := os.ReadFile(filepath.Join(root, "mancodes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document manCodeDocument
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if document.Version != manCodeFileVersion || len(document.Groups) != 1 {
		t.Fatalf("unexpected catalog document: %s", data)
	}
	reloaded, err := NewFileManCodeRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	groups, err := reloaded.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].ID != created.ID || groups[0].Name != "保健" {
		t.Fatalf("reloaded catalog = %#v", groups)
	}
	if !slices.Equal(groups[0].Codes, []string{"123", "456", "789"}) {
		t.Fatalf("reloaded codes = %#v", groups[0].Codes)
	}
	replaced, err := app.ReplaceManCodeGroupCodes(ReplaceManCodeGroupCodesRequest{
		ID:    created.ID,
		Codes: []string{"10", "2, 10", "sku-a"},
		Raw:   "SKU-B，2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(replaced.Codes, []string{"2", "10", "sku-a", "SKU-B"}) {
		t.Fatalf("replaced codes = %#v", replaced.Codes)
	}
}

func TestSaveManCodeGroupRejectsDuplicateName(t *testing.T) {
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{})
	first, err := app.SaveManCodeGroup(SaveManCodeGroupRequest{Name: "保健", Codes: []string{"123"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.SaveManCodeGroup(SaveManCodeGroupRequest{Name: "保健", Codes: []string{"456"}}); err == nil {
		t.Fatal("expected duplicate group name to be rejected")
	}
	renamed, err := app.SaveManCodeGroup(SaveManCodeGroupRequest{ID: first.ID, Name: "保健品"})
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "保健品" || !slices.Equal(renamed.Codes, []string{"123"}) {
		t.Fatalf("rename changed unexpected fields: %#v", renamed)
	}
	second, err := app.SaveManCodeGroup(SaveManCodeGroupRequest{Name: "保健", Raw: "789"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.SaveManCodeGroup(SaveManCodeGroupRequest{ID: second.ID, Name: "保健品"}); err == nil {
		t.Fatal("expected rename onto an existing name to be rejected")
	}
	groups, err := app.ListManCodeGroups()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || groups[0].Name != "保健品" || groups[1].Name != "保健" {
		t.Fatalf("rejected rename changed catalog: %#v", groups)
	}
}

func TestDeleteManCodeGroupMissingID(t *testing.T) {
	app, _, _ := newTestApp(t, new(fakeEngine), fakeClients{})
	created, err := app.SaveManCodeGroup(SaveManCodeGroupRequest{Name: "保健"})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.DeleteManCodeGroup("00000000-0000-4000-8000-000000000000"); err == nil {
		t.Fatal("expected missing id to be rejected")
	}
	if err := app.DeleteManCodeGroup(""); err == nil {
		t.Fatal("expected empty id to be rejected")
	}
	if err := app.DeleteManCodeGroup(created.ID); err != nil {
		t.Fatal(err)
	}
	groups, err := app.ListManCodeGroups()
	if err != nil || len(groups) != 0 {
		t.Fatalf("deleted catalog = %#v, %v", groups, err)
	}
	if err := app.DeleteManCodeGroup(created.ID); err == nil {
		t.Fatal("expected deleting an already removed id to fail")
	}
}
