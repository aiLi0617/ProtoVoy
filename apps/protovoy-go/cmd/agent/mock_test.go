package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAgentMockManagement(t *testing.T) {
	const token = "fixture-agent-mock-management-token"
	agent, err := newAgent("127.0.0.1:39217", token, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	api := agent.routes()
	call := func(method, credential string, body []byte, status int) []byte {
		t.Helper()
		request := httptest.NewRequest(method, "http://127.0.0.1:39217/v1/mock", bytes.NewReader(body))
		request.RemoteAddr = "127.0.0.1:45000"
		if credential != "" {
			request.Header.Set("Authorization", "Bearer "+credential)
		}
		response := httptest.NewRecorder()
		api.ServeHTTP(response, request)
		if response.Code != status {
			t.Fatalf("%s returned %d %s, want %d", method, response.Code, response.Body, status)
		}
		return response.Body.Bytes()
	}
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		call(method, "", nil, http.StatusUnauthorized)
	}
	rules := []byte(`[{"id":"fixture","method":"GET","path":"/hello","status":201,"body":"Agent mock response"}]`)
	call(http.MethodPost, token, []byte("["), http.StatusBadRequest)
	body := call(http.MethodPost, token, rules, http.StatusOK)
	defer call(http.MethodDelete, token, nil, http.StatusOK)
	var state struct {
		Running bool
		Address string
	}
	if err := json.Unmarshal(body, &state); err != nil || !state.Running || state.Address == "" {
		t.Fatalf("invalid running state: %s %v", body, err)
	}
	call(http.MethodPost, token, rules, http.StatusConflict)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+state.Address+"/m1/default-project/default/hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 201 || string(data) != "Agent mock response" {
		t.Fatalf("unexpected Mock response: %d %q %v", response.StatusCode, data, err)
	}
	body = call(http.MethodDelete, token, nil, http.StatusOK)
	if err := json.Unmarshal(body, &state); err != nil || state.Running || state.Address != "" {
		t.Fatalf("invalid stopped state: %s %v", body, err)
	}
}
