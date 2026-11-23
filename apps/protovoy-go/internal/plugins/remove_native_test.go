//go:build !js

package plugins

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemovalConfinesCleanupAndAllowsReinstallation(t *testing.T) {
	store, outside := t.TempDir(), t.TempDir()
	manifest := []byte(`{"id":"fixture","name":"Fixture","version":"1","kind":"transformer"}`)
	component := []byte("\x00asm\x0d\x00\x01\x00")
	trust := Trust{AllowUnsigned: true}
	if _, err := InstallPackageBytes(store, manifest, component, trust); err != nil {
		t.Fatal(err)
	}
	if err := SetEnabled(store, "fixture", false); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(outside, "keep.txt")
	if err := os.WriteFile(sentinel, []byte("keep external data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(store, "fixture", "external")); err != nil {
		t.Logf("nested symlink case unavailable: %v", err)
	}
	for _, id := range []string{"../fixture", "", "fixture/.."} {
		if _, err := RemoveInstalled(store, id); err == nil {
			t.Fatal("accepted removal path traversal")
		}
	}
	result, err := RemoveInstalled(store, "fixture")
	if err != nil || result.ID != "fixture" || result.CleanupPending {
		t.Fatalf("removal failed: %v, %v", result, err)
	}
	if data, err := os.ReadFile(sentinel); err != nil || string(data) != "keep external data" {
		t.Fatalf("removal changed external data: %v", err)
	}
	entries, err := os.ReadDir(store)
	if err != nil || len(entries) != 0 {
		t.Fatalf("removal left active or staged files: %v, %v", entries, err)
	}
	if _, err := ExecuteInstalled(t.Context(), store, "fixture", trust, "transformer", ""); err == nil {
		t.Fatal("removed plugin remained executable")
	}
	if _, err := InstallPackageBytes(store, manifest, component, trust); err != nil {
		t.Fatalf("could not reinstall removed ID: %v", err)
	}
	items, err := Inventory(store)
	if err != nil || len(items) != 1 || items[0].Disabled {
		t.Fatalf("reinstallation retained removed package state: %v", err)
	}
	broken := filepath.Join(store, "broken")
	if err := os.Mkdir(broken, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "manifest.json"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := RemoveInstalled(store, "broken"); err != nil {
		t.Fatalf("could not remove malformed package: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(store, "alias")); err == nil {
		if _, err := RemoveInstalled(store, "alias"); err == nil {
			t.Fatal("accepted external package symlink as installed directory")
		}
		if data, err := os.ReadFile(sentinel); err != nil || string(data) != "keep external data" {
			t.Fatal("external package symlink caused data loss")
		}
	}
}
