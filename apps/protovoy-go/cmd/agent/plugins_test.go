package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"protovoy.dev/protovoy/internal/agentclient"
)

func TestPluginInventoryRequiresPairingAndSupportsClient(t *testing.T) {
	const token = "fixture-plugin-inventory-pairing-token"
	agent, err := newAgent("127.0.0.1:39217", token, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	agent.pluginDirectory = t.TempDir()
	directory := filepath.Join(agent.pluginDirectory, "fixture")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "manifest.json"),
		[]byte(`{"id":"fixture","name":"Fixture","version":"1","kind":"transformer"}`), 0600); err != nil {
		t.Fatal(err)
	}
	api := agent.routes()
	do := func(request *http.Request) (*http.Response, error) {
		request.RemoteAddr = "127.0.0.1:45000"
		response := httptest.NewRecorder()
		api.ServeHTTP(response, request)
		return response.Result(), nil
	}
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:39217/v1/plugins", nil)
	response, err := do(request)
	if err != nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatal("plugin inventory bypassed authentication")
	}
	response.Body.Close()
	client, err := agentclient.Pair(t.Context(), "127.0.0.1:39217", token, do)
	if err != nil || !client.SupportsPlugins() {
		t.Fatalf("plugin capability handshake failed: %v", err)
	}
	items, err := client.Plugins(t.Context())
	if err != nil || len(items) != 1 || items[0].Manifest == nil || items[0].Manifest.Name != "Fixture" {
		t.Fatalf("client inventory failed: %v, %v", items, err)
	}
}
