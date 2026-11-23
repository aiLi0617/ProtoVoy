package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHostedHTTPUsesRequestTimeout(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(200 * time.Millisecond):
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer upstream.Close()
	const addr = "127.0.0.1:39300"
	api := handler(loopbackAccess(addr), t.TempDir(), upstream.URL, "")
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"method":"GET","timeoutMs":5}`, http.StatusGatewayTimeout},
		{`{"method":"GET","timeoutMs":2000}`, http.StatusOK},
		{`{"method":"GET","timeoutMs":300001}`, http.StatusBadRequest},
		{`{"method":"GET","eventStream":true,"timeoutMs":5}`, http.StatusBadRequest},
	} {
		req := httptest.NewRequest(http.MethodPost, "http://"+addr+"/api/preview", strings.NewReader(tc.body))
		req.Header.Set("Origin", "http://"+addr)
		req.Header.Set("X-ProtoVoy-Preview", "1")
		w := httptest.NewRecorder()
		api.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("got HTTP %d, want %d for %s", w.Code, tc.status, tc.body)
		}
	}
}
