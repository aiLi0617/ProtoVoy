//go:build !js

package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestInstalledOperationsRejectDirectoryAliasInsideStore(t *testing.T) {
	store := t.TempDir()
	trust := Trust{AllowUnsigned: true}
	manifest := []byte(`{"id":"fixture","name":"Fixture","version":"1","kind":"transformer"}`)
	if _, err := InstallPackageBytes(store, manifest, []byte("\x00asm\x0d\x00\x01\x00"), trust); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(store, "fixture"), filepath.Join(store, ".staged")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".staged", filepath.Join(store, "fixture")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := SetEnabled(store, "fixture", false); err == nil {
		t.Fatal("changed enablement through a directory alias")
	}
	if _, err := os.Stat(filepath.Join(store, ".staged", ".disabled")); !os.IsNotExist(err) {
		t.Fatalf("unexpected staged package state: %v", err)
	}
	if _, err := ExecuteInstalled(t.Context(), store, "fixture", trust, "transformer", ""); err == nil ||
		!strings.Contains(err.Error(), "installed plugin is not a directory") {
		t.Fatalf("execution should reject the alias before loading the package: %v", err)
	}
}

func TestInstallSnapshotRequiresTrustAndPreservesExistingPackage(t *testing.T) {
	source, store := t.TempDir(), filepath.Join(t.TempDir(), "plugins")
	manifest := []byte(`{"id":"example","name":"Example","version":"1","kind":"transformer","permissions":["network"]}`)
	component := []byte("\x00asm\x0d\x00\x01\x00")
	for name, data := range map[string][]byte{"manifest.json": manifest, "plugin.wasm": component, "untrusted-extra": []byte("do not copy")} {
		if err := os.WriteFile(filepath.Join(source, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := InstallPackage(store, source, Trust{AllowUnsigned: true}); err == nil {
		t.Fatal("installed without declared permission grant")
	}
	if _, err := os.Stat(store); !os.IsNotExist(err) {
		t.Fatal("untrusted installation changed store")
	}
	trust := Trust{AllowUnsigned: true, Permissions: map[string]bool{"network": true}}
	var group sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		group.Go(func() {
			_, err := InstallPackage(store, source, trust)
			results <- err
		})
	}
	group.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent install successes = %d, want 1", successes)
	}
	installed := filepath.Join(store, "example")
	pack, err := ReadPackage(installed, trust)
	if err != nil || string(pack.Component) != string(component) {
		t.Fatalf("installed snapshot is invalid: %v", err)
	}
	if _, err := os.Stat(filepath.Join(installed, "untrusted-extra")); !os.IsNotExist(err) {
		t.Fatal("copied unrelated source file")
	}
	if err := os.WriteFile(filepath.Join(source, "plugin.wasm"), append(component, 1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallPackage(store, source, trust); err == nil {
		t.Fatal("overwrote installed plugin")
	}
	unchanged, err := ReadPackage(installed, trust)
	if err != nil || unchanged.SHA256 != pack.SHA256 {
		t.Fatalf("duplicate installation changed snapshot: %v", err)
	}
	entries, err := os.ReadDir(store)
	if err != nil || len(entries) != 1 || entries[0].Name() != "example" {
		t.Fatalf("installation left staging debris: %v, %v", entries, err)
	}
}
