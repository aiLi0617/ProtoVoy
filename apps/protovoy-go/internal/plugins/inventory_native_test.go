//go:build !js

package plugins

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInventoryIsBoundedMetadataAndReportsBrokenPackages(t *testing.T) {
	store := t.TempDir()
	if items, err := Inventory(filepath.Join(store, "missing")); err != nil || len(items) != 0 {
		t.Fatalf("missing store should be empty: %v", err)
	}
	for name, manifest := range map[string]string{
		"valid":           `{"id":"valid","name":"Valid","version":"1","kind":"transformer"}`,
		"mismatch":        `{"id":"other","name":"Other","version":"1","kind":"auth"}`,
		"broken":          `{`,
		".install-hidden": `{`,
	} {
		directory := filepath.Join(store, name)
		if err := os.Mkdir(directory, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "manifest.json"), []byte(manifest), 0600); err != nil {
			t.Fatal(err)
		}
	}
	items, err := Inventory(store)
	if err != nil || len(items) != 3 {
		t.Fatalf("unexpected inventory: %v, %v", items, err)
	}
	if items[0].ID != "broken" || items[0].Error == "" || items[1].ID != "mismatch" || items[1].Error == "" ||
		items[2].ID != "valid" || items[2].Manifest == nil || items[2].Error != "" {
		t.Fatalf("inventory omitted failures or metadata: %v", items)
	}
	// Inventory must not read arbitrary files through package manifest symlinks.
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"id":"valid","name":"Outside","version":"1","kind":"auth"}`), 0600); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(store, "valid", "manifest.json")
	if err := os.Remove(manifest); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, manifest); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	items, err = Inventory(store)
	if err != nil || len(items) != 3 || items[2].Manifest != nil || items[2].Error == "" {
		t.Fatalf("inventory read outside store: %v", err)
	}
}
