package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kjanat/wacogo/wasmtools"
	"protovoy.dev/protovoy/internal/agentclient"
	"protovoy.dev/protovoy/internal/plugins"
)

func TestPluginExecutionChecksTrustAndInstalledIdentity(t *testing.T) {
	const token = "fixture-plugin-execution-pairing-token"
	agent, err := newAgent("127.0.0.1:39217", token, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	agent.pluginDirectory = t.TempDir()
	source := t.TempDir()
	tool, err := wasmtools.New(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tool.Close(t.Context())
	wat, err := os.ReadFile("../../internal/plugins/testdata/echo.wat")
	if err != nil {
		t.Fatal(err)
	}
	component, err := tool.Parse(t.Context(), wat)
	if err != nil {
		t.Fatal(err)
	}
	manifest := `{"id":"echo","name":"Echo","version":"1","kind":"transformer","permissions":["network"]}`
	for name, data := range map[string][]byte{"manifest.json": []byte(manifest), "plugin.wasm": component} {
		if err := os.WriteFile(filepath.Join(source, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := plugins.InstallPackage(agent.pluginDirectory, source, plugins.Trust{AllowUnsigned: true, Permissions: map[string]bool{"network": true}}); err != nil {
		t.Fatal(err)
	}
	api := agent.routes()
	do := func(request *http.Request) (*http.Response, error) {
		request.RemoteAddr = "127.0.0.1:45000"
		response := httptest.NewRecorder()
		api.ServeHTTP(response, request)
		return response.Result(), nil
	}
	call := func(body, credential string, status int) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:39217/v1/plugins/echo/run", strings.NewReader(body))
		if credential != "" {
			request.Header.Set("Authorization", "Bearer "+credential)
		}
		response, err := do(request)
		if err != nil || response.StatusCode != status {
			t.Fatalf("unexpected execution status: %v, %v", response, err)
		}
		response.Body.Close()
	}
	call(`{}`, "", http.StatusUnauthorized)
	call(`{"unknown":true}`, token, http.StatusBadRequest)
	call(`{} {}`, token, http.StatusBadRequest)
	call(`{"kind":"transformer","allowUnsigned":true}`, token, http.StatusBadRequest)
	call(`{"kind":"transformer","permissions":["network"]}`, token, http.StatusBadRequest)
	client, err := agentclient.Pair(t.Context(), "127.0.0.1:39217", token, do)
	if err != nil {
		t.Fatal(err)
	}
	input := plugins.RunRequest{Kind: "transformer", Input: "你好\nfrom paired client", AllowUnsigned: true, Permissions: []string{"network"}}
	stale := input
	stale.ExpectedVersion = "stale-version"
	if _, err := client.RunPlugin(t.Context(), "echo", stale); err == nil || !strings.Contains(err.Error(), "version changed") {
		t.Fatalf("stale plugin selection was not rejected: %v", err)
	}
	output, err := client.RunPlugin(t.Context(), "echo", input)
	if err != nil || output != input.Input {
		t.Fatalf("paired plugin roundtrip failed: %v", err)
	}
	for _, test := range []struct {
		body, credential string
		status           int
	}{
		{`{"enabled":false}`, "", http.StatusUnauthorized},
		{`{}`, token, http.StatusBadRequest},
		{`{"enabled":null}`, token, http.StatusBadRequest},
		{`{"enabled":false,"unknown":true}`, token, http.StatusBadRequest},
		{`{"enabled":false} {}`, token, http.StatusBadRequest},
	} {
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:39217/v1/plugins/echo/state", strings.NewReader(test.body))
		if test.credential != "" {
			request.Header.Set("Authorization", "Bearer "+test.credential)
		}
		response, err := do(request)
		if err != nil || response.StatusCode != test.status {
			t.Fatalf("unexpected state response: %v, %v", response, err)
		}
		response.Body.Close()
	}
	if err := client.SetPluginEnabled(t.Context(), "echo", false); err != nil {
		t.Fatal(err)
	}
	if _, err := client.RunPlugin(t.Context(), "echo", input); err == nil || !strings.Contains(err.Error(), "plugin is disabled") {
		t.Fatalf("disabled plugin executed: %v", err)
	}
	items, err := client.Plugins(t.Context())
	if err != nil || len(items) != 1 || !items[0].Disabled {
		t.Fatalf("disabled state not visible: %v", err)
	}
	if err := client.SetPluginEnabled(t.Context(), "echo", true); err != nil {
		t.Fatal(err)
	}
	input.AllowUnsigned = false
	if _, err := client.RunPlugin(t.Context(), "echo", input); err == nil {
		t.Fatal("enabling bypassed signature policy")
	}
	input.AllowUnsigned = true
	if output, err := client.RunPlugin(t.Context(), "echo", input); err != nil || output != input.Input {
		t.Fatalf("enabled plugin did not resume: %v", err)
	}
	for _, id := range []string{"../echo", "echo/other", ""} {
		if _, err := client.RunPlugin(t.Context(), id, input); err == nil {
			t.Fatal("accepted invalid installed plugin ID")
		}
	}
	encoded, _ := json.Marshal(input)
	agent.pluginMu.Lock()
	call(string(encoded), token, http.StatusTooManyRequests)
	agent.pluginMu.Unlock()
	if err := os.WriteFile(filepath.Join(agent.pluginDirectory, "echo", "manifest.json"),
		[]byte(strings.Replace(manifest, `"id":"echo"`, `"id":"changed"`, 1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := client.RunPlugin(t.Context(), "echo", input); err == nil {
		t.Fatal("executed changed installed identity")
	}
}
