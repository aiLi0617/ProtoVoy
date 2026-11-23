//go:build !js

package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUploadedPackageRejectsPathsCoreModulesAndOversizedStoredManifest(t *testing.T) {
	for _, test := range []struct {
		manifest  string
		component []byte
	}{
		{`{"id":"../outside","name":"X","version":"1","kind":"auth"}`, []byte("\x00asm\x0d\x00\x01\x00")},
		{`{"id":"fixture","name":"X","version":"1","kind":"auth"}`, []byte("\x00asm\x01\x00\x00\x00")},
		{`{"id":"fixture","name":"` + strings.Repeat("&", 20000) + `","version":"1","kind":"auth"}`, []byte("\x00asm\x0d\x00\x01\x00")},
	} {
		store := filepath.Join(t.TempDir(), "store")
		if _, err := InstallPackageBytes(store, []byte(test.manifest), test.component, Trust{AllowUnsigned: true}); err == nil {
			t.Fatal("accepted unsafe package upload")
		}
		if _, err := os.Stat(store); !os.IsNotExist(err) {
			t.Fatal("rejected upload changed plugin store")
		}
	}
}
