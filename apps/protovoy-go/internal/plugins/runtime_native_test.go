//go:build !js

package plugins

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kjanat/wacogo/wasmtools"
)

func TestExecutePackageThroughTrustAndComponentRuntime(t *testing.T) {
	ctx := t.Context()
	tool, err := wasmtools.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tool.Close(ctx)
	component, err := tool.Parse(ctx, []byte(`(component
		(core module $m
			(memory (export "memory") 1)
			(global $bump (mut i32) (i32.const 1024))
			(func (export "realloc") (param i32 i32 i32 i32) (result i32)
				(local $p i32)
				global.get $bump local.set $p
				global.get $bump local.get 3 i32.add global.set $bump local.get $p)
			(func (export "echo") (param i32 i32) (result i32)
				i32.const 0 local.get 0 i32.store
				i32.const 4 local.get 1 i32.store i32.const 0))
		(core instance $i (instantiate $m))
		(func (export "transform") (param "input" string) (result string)
			(canon lift (core func $i "echo") (memory $i "memory") (realloc (func $i "realloc")))))`))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "manifest.json"),
		[]byte(`{"id":"echo","name":"Echo","version":"1","kind":"transformer"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "plugin.wasm"), component, 0600); err != nil {
		t.Fatal(err)
	}
	trust := Trust{AllowUnsigned: true}
	for _, input := range []string{"hello", "你好", ""} {
		output, err := ExecutePackage(ctx, root, trust, "transformer", input)
		if err != nil || output != input {
			t.Fatalf("component round trip failed: %v", err)
		}
	}
	if _, err := ExecutePackage(ctx, root, Trust{}, "transformer", ""); err == nil {
		t.Fatal("executed unsigned package without explicit policy")
	}
	if _, err := ExecutePackage(ctx, root, trust, "auth", ""); err == nil {
		t.Fatal("executed package with mismatched kind")
	}
	if _, err := ExecutePackage(ctx, root, trust, "transformer", strings.Repeat("x", (1<<20)+1)); err == nil {
		t.Fatal("accepted oversized plugin input")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := ExecutePackage(cancelled, root, trust, "transformer", ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	t.Run("interrupt running guest", func(t *testing.T) {
		binary, err := tool.Parse(ctx, []byte(`(component
			(core module $m
				(func $start (loop $forever br $forever))
				(start $start))
			(core instance $i (instantiate $m)))`))
		if err != nil {
			t.Fatal(err)
		}
		pack := Package{Manifest: Manifest{Kind: "transformer"}, Component: binary}
		guard, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		output, err := executePackage(guard, pack, "transformer", "")
		if output != "" || !errors.Is(err, context.DeadlineExceeded) ||
			!strings.Contains(err.Error(), "plugin execution timed out") || guard.Err() != nil {
			t.Fatalf("guest did not stop at the sandbox deadline: %q, %v", output, err)
		}
	})
	for _, test := range []struct {
		name, source string
	}{
		{"aggregate memory", `(component
			(core module $m (memory 512))
			(core instance $a (instantiate $m))
			(core instance $b (instantiate $m)))`},
		{"unbounded table", `(component
			(core module $m (table 1 funcref))
			(core instance $i (instantiate $m)))`},
		{"missing export", `(component)`},
		{"wrong result type", `(component
			(core module $m (func (export "f") (param i32 i32) (result i32) i32.const 0)
				(memory (export "memory") 1)
				(func (export "alloc") (param i32 i32 i32 i32) (result i32) i32.const 32))
			(core instance $i (instantiate $m))
			(func (export "transform") (param "input" string) (result u32)
				(canon lift (core func $i "f") (memory $i "memory") (realloc (func $i "alloc")))))`},
	} {
		t.Run(test.name, func(t *testing.T) {
			binary, err := tool.Parse(ctx, []byte(test.source))
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "plugin.wasm")
			if err := os.WriteFile(path, binary, 0600); err != nil {
				t.Fatal(err)
			}
			if output, err := ExecutePackage(ctx, root, trust, "transformer", ""); err == nil || output != "" {
				t.Fatal("invalid or resource-exhausting component returned successful output")
			}
			if err := os.WriteFile(path, component, 0600); err != nil {
				t.Fatal(err)
			}
			if output, err := ExecutePackage(ctx, root, trust, "transformer", "recovered"); err != nil || output != "recovered" {
				t.Fatalf("valid execution failed after component rejection: %v", err)
			}
		})
	}
}
