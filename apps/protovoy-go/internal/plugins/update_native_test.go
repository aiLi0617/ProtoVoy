//go:build !js

package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kjanat/wacogo/wasmtools"
)

func TestUpdatePublishesVerifiedSnapshotAndPreservesEnablement(t *testing.T) {
	store, source := t.TempDir(), t.TempDir()
	trust := Trust{AllowUnsigned: true}
	manifest := func(version, kind string) []byte {
		return []byte(fmt.Sprintf(`{"id":"echo","name":"Echo","version":%q,"kind":%q}`, version, kind))
	}
	if _, err := InstallPackageBytes(store, manifest("1", "transformer"), []byte("\x00asm\x0d\x00\x01\x00"), trust); err != nil {
		t.Fatal(err)
	}
	if err := SetEnabled(store, "echo", false); err != nil {
		t.Fatal(err)
	}
	tool, err := wasmtools.New(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tool.Close(t.Context())
	wat, err := os.ReadFile("testdata/echo.wat")
	if err != nil {
		t.Fatal(err)
	}
	component, err := tool.Parse(t.Context(), wat)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "plugin.wasm"), component, 0600); err != nil {
		t.Fatal(err)
	}
	writeManifest := func(version, kind string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(source, "manifest.json"), manifest(version, kind), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, version := range []string{"2", "3"} {
		writeManifest(version, "transformer")
		if _, err := UpdatePackage(store, source, trust); err != nil {
			t.Fatal(err)
		}
	}
	writeManifest("4", "auth")
	if _, err := UpdatePackage(store, source, trust); err == nil {
		t.Fatal("accepted a change to the installed plugin kind")
	}
	writeManifest("4", "transformer")
	if _, err := UpdatePackage(store, source, Trust{}); err == nil {
		t.Fatal("accepted an untrusted update")
	}
	items, err := Inventory(store)
	if err != nil || len(items) != 1 || items[0].Manifest == nil || items[0].Manifest.Version != "3" || !items[0].Disabled {
		t.Fatalf("unexpected inventory: %+v, %v", items, err)
	}
	if _, err := ExecuteInstalled(t.Context(), store, "echo", trust, "transformer", "hello"); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("update reset enablement: %v", err)
	}
	if err := SetEnabled(store, "echo", true); err != nil {
		t.Fatal(err)
	}
	request := RunRequest{Kind: "transformer", Input: "hello", ExpectedVersion: "1"}
	if _, err := ExecuteInstalledSelection(t.Context(), store, "echo", trust, request); err == nil ||
		!strings.Contains(err.Error(), "version changed") {
		t.Fatalf("stale selection reached the updated component: %v", err)
	}
	request.ExpectedVersion, request.ExpectedRevision = "3", items[0].Revision
	if request.ExpectedRevision == "" {
		t.Fatal("inventory omitted the active revision")
	}
	output, err := ExecuteInstalledSelection(t.Context(), store, "echo", trust, request)
	if err != nil || output != "hello" {
		t.Fatalf("updated snapshot did not run: %q, %v", output, err)
	}
	if _, err := RemoveInstalled(store, "echo"); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallPackageBytes(store, manifest("3", "transformer"), component, trust); err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteInstalledSelection(t.Context(), store, "echo", trust, request); err == nil ||
		!strings.Contains(err.Error(), "snapshot changed") {
		t.Fatalf("stale selection ran a reinstalled copy of the same version: %v", err)
	}
}
