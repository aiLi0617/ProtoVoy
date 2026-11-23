package main

import (
	"net/http"
	"net/http/httptest"
	"protovoy.dev/protovoy/internal/execution"
	"strings"
	"testing"
)

func TestHostedRedisReferenceAssertionsAreCheckedBeforeConnection(t *testing.T) {
	const origin = "http://127.0.0.1:39300"
	handler := redisHandler(origin, "broker.example:6380", execution.RedisOptions{Username: "reader", Password: "fixture-private-password", Database: "3", TLS: true, ServerName: "broker.example"}, "jobs")
	for _, test := range []struct {
		fields string
		status int
	}{
		{`"expectedPasswordRef":"other"`, 400},
		{`"expectedPasswordRef":"jobs","expectedUsername":"other"`, 409},
		{`"expectedPasswordRef":"jobs","expectedDatabase":"4"`, 409},
		{`"expectedPasswordRef":"jobs","expectedServerName":"other.example"`, 409},
		{`"expectedDatabase":"3"`, 400},
	} {
		request := httptest.NewRequest(http.MethodPost, origin+"/api/redis", strings.NewReader(`{"commands":[["PING"]],`+test.fields+`}`))
		request.Header.Set("Origin", origin)
		request.Header.Set("X-ProtoVoy-Preview", "1")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.status || strings.Contains(response.Body.String(), "fixture-private-password") {
			t.Fatalf("credential mismatch returned HTTP %d, want %d", response.Code, test.status)
		}
	}
}
