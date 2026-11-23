package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const fixtureWebPassword = "generated-password-for-test"

func TestLoadWebPassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web-password")
	if err := os.WriteFile(path, []byte(fixtureWebPassword+"\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	password, err := loadWebPassword("", path)
	if err != nil || password != fixtureWebPassword {
		t.Fatal("could not load the password from a CRLF-terminated file")
	}
	for _, source := range []struct{ password, path string }{
		{fixtureWebPassword, path},
		{"", path + ".missing"},
	} {
		if _, err := loadWebPassword(source.password, source.path); err == nil {
			t.Fatal("expected invalid password source to fail")
		}
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 4097)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadWebPassword("", path); err == nil {
		t.Fatal("expected oversized password file to fail")
	}
}

func TestWebHealthTracksWorkbenchAssets(t *testing.T) {
	const origin = "http://127.0.0.1:39300"
	access, err := configureWebAccess("127.0.0.1:39300", "", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	web := t.TempDir()
	for _, file := range []string{webIndex, webLoader, webModule} {
		if err := os.WriteFile(filepath.Join(web, file), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	api := handler(access, web, "", "")
	check := func(want int) {
		t.Helper()
		w := httptest.NewRecorder()
		api.ServeHTTP(w, httptest.NewRequest(http.MethodGet, origin+"/healthz", nil))
		if w.Code != want || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("health status=%d, cache control=%q; want %d and no-store", w.Code, w.Header().Get("Cache-Control"), want)
		}
	}
	check(http.StatusNoContent)
	module := filepath.Join(web, webModule)
	if err := os.Remove(module); err != nil {
		t.Fatal(err)
	}
	check(http.StatusServiceUnavailable)
	if err := os.WriteFile(module, []byte("restored fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	check(http.StatusNoContent)
}

func TestConfigureWebAccessRequiresExplicitAuthenticatedHTTPS(t *testing.T) {
	for _, test := range []struct {
		name, addr, origin, cert, key, user, password string
		valid                                         bool
	}{
		{name: "default loopback", addr: "127.0.0.1:39300", valid: true},
		{name: "IPv6 loopback", addr: "[::1]:39300", valid: true},
		{name: "remote plaintext", addr: "0.0.0.0:39300"},
		{name: "remote HTTPS", addr: "0.0.0.0:39300", origin: "https://workbench.example:39300", cert: "cert", key: "key", user: "operator", password: fixtureWebPassword, valid: true},
		{name: "origin contains path", addr: "0.0.0.0:39300", origin: "https://workbench.example/app", cert: "cert", key: "key", user: "operator", password: fixtureWebPassword},
		{name: "origin contains credentials", addr: "0.0.0.0:39300", origin: "https://user:pass@workbench.example", cert: "cert", key: "key", user: "operator", password: fixtureWebPassword},
		{name: "wrong scheme", addr: "0.0.0.0:39300", origin: "http://workbench.example", cert: "cert", key: "key", user: "operator", password: fixtureWebPassword},
		{name: "missing key", addr: "0.0.0.0:39300", origin: "https://workbench.example", cert: "cert", user: "operator", password: fixtureWebPassword},
		{name: "missing credentials", addr: "0.0.0.0:39300", origin: "https://workbench.example", cert: "cert", key: "key"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := configureWebAccess(test.addr, test.origin, test.cert, test.key, test.user, test.password)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v, got %v", test.valid, err)
			}
		})
	}
}

func TestHostedWebAuthenticatesAssetsAndExecution(t *testing.T) {
	const host = "workbench.example"
	const origin = "https://" + host
	access, err := configureWebAccess("0.0.0.0:39300", origin, "cert", "key", "operator", fixtureWebPassword)
	if err != nil {
		t.Fatal(err)
	}
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		if r.Header.Get("Authorization") != "" {
			t.Error("workbench credentials were forwarded upstream")
		}
		_, _ = io.WriteString(w, "upstream")
	}))
	defer upstream.Close()
	web := t.TempDir()
	for _, file := range []string{webIndex, webLoader, webModule, "tls-key.pem"} {
		if err := os.WriteFile(filepath.Join(web, file), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	api := handler(access, web, upstream.URL, "", socketTargets{})
	for _, test := range []struct {
		name, method, url, requestOrigin, password string
		want                                       int
	}{
		{"anonymous page", http.MethodGet, origin + "/", "", "", 401},
		{"anonymous health check", http.MethodGet, origin + "/healthz", "", "", 401},
		{"authorized health check", http.MethodGet, origin + "/healthz", "", fixtureWebPassword, 204},
		{"anonymous WASM", http.MethodGet, origin + "/protovoy.wasm", "", "", 401},
		{"anonymous HTTP execution", http.MethodPost, origin + "/api/preview", origin, "", 401},
		{"anonymous socket execution", http.MethodPost, origin + "/api/socket", origin, "", 401},
		{"anonymous Redis execution", http.MethodPost, origin + "/api/redis", origin, "", 401},
		{"anonymous gRPC execution", http.MethodPost, origin + "/api/grpc", origin, "", 401},
		{"anonymous websocket", http.MethodGet, origin + "/api/websocket", origin, "", 401},
		{"wrong password", http.MethodGet, origin + "/", "", "invalid", 401},
		{"plain HTTP with spoofed forwarding headers", http.MethodGet, "http://" + host + "/", "", fixtureWebPassword, 403},
		{"wrong host", http.MethodGet, "https://other.example/", "", fixtureWebPassword, 403},
		{"wrong origin", http.MethodPost, origin + "/api/preview", "https://other.example", fixtureWebPassword, 403},
		{"wrong WebSocket origin", http.MethodGet, origin + "/api/websocket", "https://other.example", fixtureWebPassword, 403},
		{"protected page", http.MethodGet, origin + "/", "", fixtureWebPassword, 200},
		{"protected WASM", http.MethodGet, origin + "/protovoy.wasm", "", fixtureWebPassword, 200},
		{"arbitrary file", http.MethodGet, origin + "/tls-key.pem", "", fixtureWebPassword, 404},
		{"authorized execution", http.MethodPost, origin + "/api/preview", origin, fixtureWebPassword, 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(test.method, test.url, strings.NewReader(`{"method":"GET"}`))
			r.Header.Set("Origin", test.requestOrigin)
			r.Header.Set("X-ProtoVoy-Preview", "1")
			r.Header.Set("X-Forwarded-Proto", "https")
			r.Header.Set("X-Forwarded-Host", host)
			if test.password != "" {
				r.SetBasicAuth("operator", test.password)
			}
			w := httptest.NewRecorder()
			api.ServeHTTP(w, r)
			if w.Code != test.want {
				t.Fatalf("status = %d, want %d", w.Code, test.want)
			}
			if test.want == 401 && w.Header().Get("WWW-Authenticate") == "" {
				t.Fatal("authentication challenge is missing")
			}
		})
	}
	if upstreamCalls.Load() != 1 {
		t.Fatalf("received %d upstream calls, want 1", upstreamCalls.Load())
	}
}
