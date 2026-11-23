package main

import (
	"os"
	"path/filepath"
	"testing"

	"protovoy.dev/protovoy/internal/execution"
)

func TestReadCAFileRejectsOversize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, make([]byte, (1<<20)+1), 0600); err != nil {
		t.Fatal(err)
	}
	if data, err := execution.ReadCAFile(path); err == nil || data != nil {
		t.Fatal("oversize CA was accepted")
	}
}
