package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"protovoy.dev/protovoy/internal/agentclient"
	"protovoy.dev/protovoy/internal/plugins"
)

func TestPluginUploadChecksTrustBeforeWritingAndRejectsReplacement(t *testing.T) {
	const token = "fixture-plugin-install-pairing-token"
	agent, err := newAgent("127.0.0.1:39217", token, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	agent.pluginDirectory = filepath.Join(t.TempDir(), "store")
	api := agent.routes()
	do := func(request *http.Request) (*http.Response, error) {
		request.RemoteAddr = "127.0.0.1:45000"
		response := httptest.NewRecorder()
		api.ServeHTTP(response, request)
		return response.Result(), nil
	}
	call := func(body []byte, credential string, status int) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:39217/v1/plugins", bytes.NewReader(body))
		if credential != "" {
			request.Header.Set("Authorization", "Bearer "+credential)
		}
		response, err := do(request)
		if err != nil || response.StatusCode != status {
			t.Fatalf("unexpected install status: %v, %v", response, err)
		}
		response.Body.Close()
	}
	input := plugins.InstallRequest{
		Manifest:  json.RawMessage(`{"id":"fixture","name":"Fixture","version":"1","kind":"transformer","permissions":["network"]}`),
		Component: []byte("\x00asm\x0d\x00\x01\x00"),
	}
	encoded, _ := json.Marshal(input)
	call(encoded, "", http.StatusUnauthorized)
	call([]byte(`{"unknown":true}`), token, http.StatusBadRequest)
	call(append(encoded, []byte(` {}`)...), token, http.StatusBadRequest)
	call(encoded, token, http.StatusBadRequest)
	client, err := agentclient.Pair(t.Context(), "127.0.0.1:39217", token, do)
	if err != nil {
		t.Fatal(err)
	}
	input.AllowUnsigned = true
	if _, err := client.InstallPlugin(t.Context(), input); err == nil || !strings.Contains(err.Error(), "permission was not granted") {
		t.Fatalf("missing grant did not yield a useful rejection: %v", err)
	}
	input.Permissions = []string{"network"}
	input.AllowUnsigned = false
	if _, err := client.InstallPlugin(t.Context(), input); err == nil || !strings.Contains(err.Error(), "trusted publisher signature") {
		t.Fatalf("unsigned package bypassed policy: %v", err)
	}
	if _, err := os.Stat(agent.pluginDirectory); !os.IsNotExist(err) {
		t.Fatal("rejected upload changed the plugin store")
	}
	input.AllowUnsigned = true
	installed, err := client.InstallPlugin(t.Context(), input)
	if err != nil || installed.ID != "fixture" || len(installed.SHA256) != 64 || installed.SignatureVerified {
		t.Fatalf("upload installation failed: %v, %v", installed, err)
	}
	input.Component = append(input.Component, 1)
	if _, err := client.InstallPlugin(t.Context(), input); err == nil || !strings.Contains(err.Error(), "already installed") {
		t.Fatalf("duplicate upload overwrote package: %v", err)
	}
	pack, err := plugins.ReadPackage(filepath.Join(agent.pluginDirectory, "fixture"), plugins.Trust{AllowUnsigned: true, Permissions: map[string]bool{"network": true}})
	if err != nil || pack.SHA256 != installed.SHA256 {
		t.Fatalf("duplicate upload changed installed bytes: %v", err)
	}
	update := input
	update.Manifest = json.RawMessage(strings.Replace(string(input.Manifest), `"version":"1"`, `"version":"2"`, 1))
	updateBody, err := json.Marshal(update)
	if err != nil {
		t.Fatal(err)
	}
	updateRequest := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:39217/v1/plugins/fixture/update", bytes.NewReader(updateBody))
	unauthorized, err := do(updateRequest)
	if err != nil || unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatal("plugin update bypassed authentication")
	}
	unauthorized.Body.Close()
	if _, err := client.UpdatePlugin(t.Context(), "different", update); err == nil {
		t.Fatal("accepted update for a different plugin")
	}
	if err := client.SetPluginEnabled(t.Context(), "fixture", false); err != nil {
		t.Fatal(err)
	}
	updated, err := client.UpdatePlugin(t.Context(), "fixture", update)
	if err != nil || updated.ID != "fixture" || updated.SHA256 == installed.SHA256 {
		t.Fatalf("paired update failed: %v, %v", updated, err)
	}
	updatedItems, err := client.Plugins(t.Context())
	if err != nil || len(updatedItems) != 1 || updatedItems[0].Manifest == nil ||
		updatedItems[0].Manifest.Version != "2" || !updatedItems[0].Disabled {
		t.Fatalf("updated inventory lost version or disablement: %v, %v", updatedItems, err)
	}
	agent.pluginMu.Lock()
	call(encoded, token, http.StatusTooManyRequests)
	agent.pluginMu.Unlock()
	request := httptest.NewRequest(http.MethodDelete, "http://127.0.0.1:39217/v1/plugins/fixture", nil)
	response, err := do(request)
	if err != nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatal("plugin removal bypassed authentication")
	}
	response.Body.Close()
	agent.pluginMu.Lock()
	if _, err := client.RemovePlugin(t.Context(), "fixture"); err == nil || !strings.Contains(err.Error(), "operation is in progress") {
		t.Fatalf("removal bypassed plugin operation slot: %v", err)
	}
	agent.pluginMu.Unlock()
	removed, err := client.RemovePlugin(t.Context(), "fixture")
	if err != nil || removed.ID != "fixture" || removed.CleanupPending {
		t.Fatalf("paired removal failed: %v", err)
	}
	items, err := client.Plugins(t.Context())
	if err != nil || len(items) != 0 {
		t.Fatalf("removed plugin remained in inventory: %v", err)
	}
	if _, err := client.RunPlugin(t.Context(), "fixture", plugins.RunRequest{Kind: "transformer", AllowUnsigned: true, Permissions: []string{"network"}}); err == nil {
		t.Fatal("removed plugin remained executable through Agent")
	}
	input.Component = input.Component[:8]
	if result, err := client.InstallPlugin(t.Context(), input); err != nil || result.SHA256 != installed.SHA256 {
		t.Fatalf("could not reinstall removed ID: %v", err)
	}
}
