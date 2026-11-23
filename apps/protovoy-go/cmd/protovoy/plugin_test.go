package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kjanat/wacogo/wasmtools"
)

func TestPluginInspectEnforcesTrustBeforeProducingOutput(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"manifest.json": `{"id":"fixture","name":"Fixture","version":"1","kind":"transformer","permissions":["network"]}`,
		"plugin.wasm":   "\x00asm\x0d\x00\x01\x00",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{root}, {"-allow-unsigned", root}, {"-grant", "network", root},
		{"-grant", "unknown", "-allow-unsigned", root},
	} {
		var output bytes.Buffer
		if err := inspectPlugin(args, "", &output); err == nil || output.Len() != 0 {
			t.Fatal("plugin inspection produced output without a valid trust policy")
		}
	}
	var output bytes.Buffer
	args := []string{"-grant", "network", "-allow-unsigned", root}
	if err := inspectPlugin(args, "", &output); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Manifest          struct{ ID string }
		SHA256            string
		SignatureVerified bool
	}
	if json.Unmarshal(output.Bytes(), &result) != nil || result.Manifest.ID != "fixture" ||
		len(result.SHA256) != 64 || result.SignatureVerified || bytes.Contains(output.Bytes(), []byte("fixture-component")) {
		t.Fatal("inspection output omitted metadata or exposed binary contents")
	}
	output.Reset()
	if err := inspectPlugin(args, `{"official":"invalid"}`, &output); err == nil || output.Len() != 0 {
		t.Fatal("invalid trusted key configuration was accepted")
	}
}

func TestPluginRunReadsInputAndWritesComponentOutput(t *testing.T) {
	tool, err := wasmtools.New(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tool.Close(t.Context())
	component, err := tool.Parse(t.Context(), []byte(`(component
		(core module $m
			(memory (export "memory") 1)
			(global $next (mut i32) (i32.const 1024))
			(func (export "alloc") (param i32 i32 i32 i32) (result i32)
				(local $p i32)
				global.get $next local.set $p
				global.get $next local.get 3 i32.add global.set $next local.get $p)
			(func (export "echo") (param i32 i32) (result i32)
				i32.const 0 local.get 0 i32.store
				i32.const 4 local.get 1 i32.store i32.const 0))
		(core instance $i (instantiate $m))
		(func (export "transform") (param "input" string) (result string)
			(canon lift (core func $i "echo") (memory $i "memory") (realloc (func $i "alloc")))))`))
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "manifest.json"),
		[]byte(`{"id":"echo","name":"Echo","version":"1","kind":"transformer"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "plugin.wasm"), component, 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"-allow-unsigned", directory}
	var output bytes.Buffer
	const input = "你好\nplugin input\n"
	if err := executePlugin(t.Context(), args, "", strings.NewReader(input), &output); err != nil || output.String() != input {
		t.Fatalf("plugin CLI round trip failed: %v", err)
	}
	for _, test := range []struct {
		args  []string
		input string
	}{
		{[]string{directory}, input},
		{args, strings.Repeat("x", (1<<20)+1)},
		{args, string([]byte{255})},
	} {
		output.Reset()
		if err := executePlugin(t.Context(), test.args, "", strings.NewReader(test.input), &output); err == nil || output.Len() != 0 {
			t.Fatal("rejected plugin execution emitted output")
		}
	}
}
