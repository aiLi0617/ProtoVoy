//go:build !js

package mock

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEmptyMockExportCanBeLoaded(t *testing.T) {
	data, err := Encode(nil)
	if err != nil || string(data) != "[]" {
		t.Fatalf("empty export = %s, %v", data, err)
	}
	path := filepath.Join(t.TempDir(), "rules.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFile(path); err != nil {
		t.Fatalf("empty export rejected: %v", err)
	}
}
