//go:build !js

package main

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"protovoy.dev/protovoy/internal/execution"
)

func TestDesktopHTTPResponseHeaderPreview(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Set-Cookie", "session=fixture")
		w.Header().Add("X-Fixture", "first")
		w.Header().Add("X-Fixture", "second")
		w.Header().Set("X-Large", strings.Repeat("x", 3000))
		_, _ = w.Write([]byte("fixture response"))
	}))
	defer server.Close()
	var first result
	value := readResponse(t.Context(), server.URL, execution.HTTPPayload{Method: "GET"}, false, func(update result) {
		if first.status == "" {
			first = update
		}
	})
	if !value.done || value.status != "HTTP 200" || value.body != "fixture response" || value.previewBytes != 16 {
		t.Fatalf("unexpected response: %+v", value)
	}
	if first.headersTime <= 0 || value.headersTime != first.headersTime || value.previewTime < value.headersTime {
		t.Fatal("invalid desktop response timing")
	}
	if string(value.rawPreview) != "fixture response" || len(first.rawPreview) != 0 {
		t.Fatal("raw preview was missing or exposed before completion")
	}
	if !slices.Equal(first.headers, value.headers) || !slices.Contains(value.headers, [2]string{"Set-Cookie", "[redacted]"}) ||
		!slices.Contains(value.headers, [2]string{"X-Fixture", "first"}) ||
		!slices.Contains(value.headers, [2]string{"X-Fixture", "second"}) ||
		slices.ContainsFunc(value.headers, func(header [2]string) bool { return header[0] == "X-Large" }) {
		t.Fatal("desktop header preview lost metadata, exposed a cookie, or exceeded its size limit")
	}
}
