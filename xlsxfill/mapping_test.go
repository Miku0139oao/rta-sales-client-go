package xlsxfill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadStoreMapJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mapping.json")
	if err := os.WriteFile(path, []byte(`{"STORE_A":"RTA_A"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	mapping, err := LoadStoreMap(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := mapping.ResolveStore("STORE_A"); !ok || got != "RTA_A" {
		t.Fatalf("resolved=%q ok=%t, want RTA_A/true", got, ok)
	}
}

func TestLoadStoreMapCSV(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mapping.csv")
	content := "sheet_store_id,rta_business_store_id\nSTORE_A,RTA_A\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	mapping, err := LoadStoreMap(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := mapping.ResolveStore("STORE_A"); !ok || got != "RTA_A" {
		t.Fatalf("resolved=%q ok=%t, want RTA_A/true", got, ok)
	}
}

func TestIdentityStoreMap(t *testing.T) {
	got, ok := (IdentityStoreMap{}).ResolveStore(" STORE_A ")
	if !ok || got != "STORE_A" {
		t.Fatalf("resolved=%q ok=%t, want STORE_A/true", got, ok)
	}
}
