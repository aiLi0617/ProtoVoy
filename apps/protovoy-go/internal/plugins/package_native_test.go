//go:build !js

package plugins

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPackageRequiresTrustAndExplicitPermissions(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "manifest.json"),
		[]byte(`{"id":"example","name":"Example","version":"1","kind":"transformer","permissions":["network"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "plugin.wasm"), []byte("\x00asm\x0d\x00\x01\x00"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPackage(directory, Trust{AllowUnsigned: true}); err == nil {
		t.Fatal("accepted ungranted permission")
	}
	trust := Trust{Permissions: map[string]bool{"network": true}}
	if _, err := ReadPackage(directory, trust); err == nil {
		t.Fatal("accepted unsigned package without explicit policy")
	}
	trust.AllowUnsigned = true
	pack, err := ReadPackage(directory, trust)
	if err != nil || pack.Manifest.ID != "example" || string(pack.Component) != "\x00asm\x0d\x00\x01\x00" ||
		len(pack.SHA256) != 64 || pack.SignatureVerified {
		t.Fatalf("unexpected package: %v", err)
	}
}

func TestPackageRejectsOversizedAndNonRegularFiles(t *testing.T) {
	for _, name := range []string{"manifest.json", "plugin.wasm"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			manifest := []byte(`{"id":"example","name":"Example","version":"1","kind":"transformer"}`)
			if err := os.WriteFile(filepath.Join(root, "manifest.json"), manifest, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "plugin.wasm"), []byte("fixture"), 0600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, name)
			file, err := os.OpenFile(path, os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			size := int64(64<<10) + 1
			if name == "plugin.wasm" {
				size = (32 << 20) + 1
			}
			err = file.Truncate(size)
			file.Close()
			if err != nil {
				t.Fatal(err)
			}
			trust := Trust{AllowUnsigned: true}
			if _, err := ReadPackage(root, trust); err == nil {
				t.Fatal("accepted oversized package file")
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
			if _, err := ReadPackage(root, trust); err == nil {
				t.Fatal("accepted directory as package file")
			}
		})
	}
}

func TestPackageRejectsComponentSymlinkInsideRoot(t *testing.T) {
	root := t.TempDir()
	manifest := []byte(`{"id":"example","name":"Example","version":"1","kind":"transformer"}`)
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), manifest, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "component.wasm"), []byte("\x00asm\x0d\x00\x01\x00"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("component.wasm", filepath.Join(root, "plugin.wasm")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if _, err := ReadPackage(root, Trust{AllowUnsigned: true}); err == nil {
		t.Fatal("accepted a symlink as a package file")
	}
}

func TestPackageCannotReadComponentOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.wasm")
	if err := os.WriteFile(outside, []byte("outside fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"),
		[]byte(`{"id":"example","name":"Example","version":"1","kind":"transformer"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "plugin.wasm")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if _, err := ReadPackage(root, Trust{AllowUnsigned: true}); err == nil {
		t.Fatal("read component through a symlink outside the package root")
	}
}
