//go:build !js

package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginStatePersistsAndBlocksBothExecutionEntrypoints(t *testing.T) {
	store := t.TempDir()
	trust := Trust{AllowUnsigned: true}
	manifest := []byte(`{"id":"fixture","name":"Fixture","version":"1","kind":"transformer"}`)
	component := []byte("\x00asm\x0d\x00\x01\x00")
	if _, err := InstallPackageBytes(store, manifest, component, trust); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := SetEnabled(store, "fixture", false); err != nil {
			t.Fatal(err)
		}
	}
	for _, run := range []func() (string, error){
		func() (string, error) {
			return ExecuteInstalled(t.Context(), store, "fixture", trust, "transformer", "")
		},
		func() (string, error) {
			return ExecutePackage(t.Context(), filepath.Join(store, "fixture"), trust, "transformer", "")
		},
	} {
		if _, err := run(); err == nil || !strings.Contains(err.Error(), "plugin is disabled") {
			t.Fatalf("disabled package reached Component validation: %v", err)
		}
	}
	items, err := Inventory(store)
	if err != nil || len(items) != 1 || !items[0].Disabled {
		t.Fatalf("inventory lost persisted disabled state: %v", err)
	}
	for range 2 {
		if err := SetEnabled(store, "fixture", true); err != nil {
			t.Fatal(err)
		}
	}
	items, err = Inventory(store)
	if err != nil || len(items) != 1 || items[0].Disabled {
		t.Fatalf("inventory lost enabled state: %v", err)
	}
	if _, err := ReadPackage(filepath.Join(store, "fixture"), trust); err != nil {
		t.Fatalf("state operation damaged package: %v", err)
	}
	if err := SetEnabled(store, "../fixture", false); err == nil {
		t.Fatal("accepted state path traversal")
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "manifest.json"), []byte(strings.Replace(string(manifest), `"id":"fixture"`, `"id":"escape"`, 1)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(store, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := SetEnabled(store, "escape", false); err == nil {
		t.Fatal("followed package symlink outside store")
	}
	if _, err := os.Stat(filepath.Join(outside, ".disabled")); !os.IsNotExist(err) {
		t.Fatal("state operation wrote outside store")
	}
}
