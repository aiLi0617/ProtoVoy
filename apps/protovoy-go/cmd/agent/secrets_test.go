package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestAgentSecretRoutesNeverReturnValue(t *testing.T) {
	keyring.MockInit()
	const token = "fixture-agent-secret-management-token"
	agent, err := newAgent("127.0.0.1:39217", token, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	api := agent.routes()
	call := func(method, credential, body string, status int) string {
		t.Helper()
		request := httptest.NewRequest(method, "http://127.0.0.1:39217/v1/secrets/fixture", strings.NewReader(body))
		request.RemoteAddr = "127.0.0.1:45000"
		if credential != "" {
			request.Header.Set("Authorization", "Bearer "+credential)
		}
		response := httptest.NewRecorder()
		api.ServeHTTP(response, request)
		if response.Code != status || strings.Contains(response.Body.String(), "private-value") {
			t.Fatalf("unexpected credential response: status=%d", response.Code)
		}
		return response.Body.String()
	}
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		call(method, "", "", http.StatusUnauthorized)
	}
	call(http.MethodPut, token, `{"value":"private-value"}`, http.StatusNoContent)
	if !strings.Contains(call(http.MethodGet, token, "", http.StatusOK), `"exists":true`) {
		t.Fatal("stored reference not found")
	}
	call(http.MethodDelete, token, "", http.StatusNoContent)
	if !strings.Contains(call(http.MethodGet, token, "", http.StatusOK), `"exists":false`) {
		t.Fatal("deleted reference still exists")
	}
}
