package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"protovoy.dev/protovoy/internal/execution"
)

func TestServerShutdownDrainsActiveHTTPRequest(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	started, release, draining := make(chan struct{}), make(chan struct{}), make(chan struct{})
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusNoContent)
	})}
	server.RegisterOnShutdown(func() { close(draining) })
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	bridge := &webSocketBridge{stopping: make(chan struct{})}
	go func() {
		finished <- serveUntilShutdown(ctx, server, func() error { return server.Serve(listener) }, bridge.stop)
	}()

	client := &http.Client{Timeout: 5 * time.Second}
	response := make(chan int, 1)
	go func() {
		res, err := client.Get("http://" + listener.Addr().String())
		if err != nil {
			response <- 0
			return
		}
		res.Body.Close()
		response <- res.StatusCode
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not reach the server")
	}
	cancel()
	select {
	case <-draining:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("shutdown did not start")
	}
	close(release)
	if status := <-response; status != http.StatusNoContent {
		t.Fatalf("in-flight request returned %d", status)
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestServerShutdownCancelsPendingSSE(t *testing.T) {
	started := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer upstream.Close()
	const origin = "http://127.0.0.1:39300"
	app := handler(loopbackAccess("127.0.0.1:39300"), t.TempDir(), upstream.URL, "")
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	request := httptest.NewRequest(http.MethodPost, origin+"/api/preview",
		strings.NewReader(`{"method":"GET","eventStream":true}`)).WithContext(ctx)
	request.Header.Set("Origin", origin)
	request.Header.Set("X-ProtoVoy-Preview", "1")
	done := make(chan struct{})
	go func() {
		defer close(done)
		app.ServeHTTP(httptest.NewRecorder(), request)
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("SSE request did not reach upstream")
	}
	app.bridge.stop()
	select {
	case <-done:
		if ctx.Err() != nil {
			t.Fatal("SSE request ran until its deadline")
		}
	case <-ctx.Done():
		t.Fatal("SSE request did not stop during shutdown")
	}
}

func TestServerShutdownClosesActiveSSE(t *testing.T) {
	upstreamClosed := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: ready\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(upstreamClosed)
	}))
	defer upstream.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	access := loopbackAccess(listener.Addr().String())
	app := handler(access, t.TempDir(), upstream.URL, "")
	server := &http.Server{Handler: app}
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	exited := make(chan error, 1)
	go func() {
		exited <- serveUntilShutdown(ctx, server, func() error { return server.Serve(listener) }, app.bridge.stop)
	}()

	client := &http.Client{Timeout: 5 * time.Second}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, access.origin+"/api/preview",
		strings.NewReader(`{"method":"GET","eventStream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", access.origin)
	request.Header.Set("X-ProtoVoy-Preview", "1")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	frame := make([]byte, len("data: ready\n\n"))
	if _, err := io.ReadFull(response.Body, frame); err != nil || string(frame) != "data: ready\n\n" {
		t.Fatalf("did not receive the live SSE event: %q, %v", frame, err)
	}
	cancel()
	if _, err := io.Copy(io.Discard, response.Body); errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("client stream stayed open until its deadline")
	}
	select {
	case err := <-exited:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not finish shutting down")
	}
	select {
	case <-upstreamClosed:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream SSE connection stayed open")
	}
}

func TestHTTPPreviewAllowsOnlyConfiguredTargets(t *testing.T) {
	var followed atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			w.Header().Set("Location", "/unlisted")
			w.WriteHeader(http.StatusFound)
			_, _ = io.WriteString(w, "redirect response")
			return
		case "/unlisted":
			followed.Store(true)
		}
		_, _ = io.WriteString(w, r.URL.RequestURI())
	}))
	defer upstream.Close()
	access := loopbackAccess("127.0.0.1:39300")
	additional := upstream.URL + "/items?page=2,3"
	targets := make(map[string]struct{})
	if err := addHostedHTTPTargets(targets, " \r\n"+additional+"\r\n"+upstream.URL+"/other\n"+upstream.URL+"/redirect"); err != nil {
		t.Fatal(err)
	}
	app := handler(access, t.TempDir(), upstream.URL+"/default", "", socketTargets{
		AdditionalHTTP: targets,
	})
	for _, test := range []struct{ target, body, upstreamStatus string }{
		{"", "/default", "200"},
		{additional, "/items?page=2,3", "200"},
		{upstream.URL + "/other", "/other", "200"},
		{upstream.URL + "/redirect", "redirect response", "302"},
		{upstream.URL + "/items?page=3", "", ""},
	} {
		r := httptest.NewRequest(http.MethodPost, access.origin+"/api/preview",
			strings.NewReader(`{"method":"GET","expectedTarget":"`+test.target+`"}`))
		r.Header.Set("Origin", access.origin)
		r.Header.Set("X-ProtoVoy-Preview", "1")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		if test.body == "" {
			if w.Code != http.StatusConflict {
				t.Fatalf("unlisted target returned %d", w.Code)
			}
		} else if w.Code != http.StatusOK || w.Body.String() != test.body {
			t.Fatalf("expected %q, got HTTP %d %q", test.body, w.Code, w.Body.String())
		}
		if got := w.Header().Get("X-ProtoVoy-Status"); got != test.upstreamStatus {
			t.Fatalf("expected upstream status %q, got %q", test.upstreamStatus, got)
		}
	}
	if followed.Load() {
		t.Fatal("hosted execution followed a redirect to an unlisted URL")
	}
}

func TestHostedHTTPTargetsRejectInvalidURLs(t *testing.T) {
	for _, value := range []string{"/relative", "ftp://example.test", "https://user@example.test", "https://example.test/#fragment"} {
		targets := make(map[string]struct{})
		if err := addHostedHTTPTargets(targets, value); err == nil || len(targets) != 0 {
			t.Errorf("accepted invalid HTTP target %q", value)
		}
	}
}

func TestCheckWebBuildRequiresAllAssets(t *testing.T) {
	web := t.TempDir()
	for _, asset := range []string{"index.html", "wasm_exec.js", "protovoy.wasm"} {
		if err := checkWebBuild(web); err == nil || !strings.Contains(err.Error(), asset) {
			t.Fatalf("missing %s was not reported: %v", asset, err)
		}
		if err := os.WriteFile(filepath.Join(web, asset), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := checkWebBuild(web); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(filepath.Join(web, "protovoy.wasm"), 0); err != nil {
		t.Fatal(err)
	}
	if err := checkWebBuild(web); err == nil || !strings.Contains(err.Error(), "protovoy.wasm") {
		t.Fatalf("empty WebAssembly file was not reported: %v", err)
	}
}

func TestWebAssetsUseNoStore(t *testing.T) {
	web := t.TempDir()
	app := handler(webAccess{host: "localhost"}, web, "", "")
	for path, asset := range map[string]string{"/": webIndex, "/" + webLoader: webLoader, "/" + webModule: webModule} {
		if err := os.WriteFile(filepath.Join(web, asset), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		app.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil))
		if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s returned status %d and Cache-Control %q", path, w.Code, w.Header().Get("Cache-Control"))
		}
	}
}

func TestHTTPPreviewAbortsTruncatedUpstreamBody(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = io.WriteString(w, "partial")
	}))
	defer upstream.Close()
	const addr = "127.0.0.1:39300"
	app := handler(loopbackAccess(addr), t.TempDir(), upstream.URL, "")
	request := httptest.NewRequest(http.MethodPost, "http://"+addr+"/api/preview", strings.NewReader(`{"method":"GET"}`))
	request.Header.Set("Origin", "http://"+addr)
	request.Header.Set("X-ProtoVoy-Preview", "1")
	defer func() {
		if recovered := recover(); recovered != http.ErrAbortHandler {
			t.Fatalf("partial response should abort the stream, got %v", recovered)
		}
	}()
	app.ServeHTTP(httptest.NewRecorder(), request)
}

func TestWebServerServesWorkbenchWithoutExecutionTargets(t *testing.T) {
	const origin = "https://workbench.example.test"
	const password = "fixture-web-password-123456"
	access, err := configureWebAccess("0.0.0.0:8443", origin, "cert.pem", "key.pem", "fixture", password)
	if err != nil {
		t.Fatal(err)
	}
	web := t.TempDir()
	if err := os.WriteFile(filepath.Join(web, "index.html"), []byte("<p>ProtoVoy</p>"), 0600); err != nil {
		t.Fatal(err)
	}
	api := handler(access, web, "", "", socketTargets{})
	for _, check := range []struct {
		method, path  string
		authenticated bool
		status        int
	}{
		{http.MethodGet, "/", false, http.StatusUnauthorized},
		{http.MethodGet, "/", true, http.StatusOK},
		{http.MethodPost, "/api/preview", true, http.StatusServiceUnavailable},
		{http.MethodGet, execution.WebSocketBridgePath, true, http.StatusServiceUnavailable},
	} {
		req := httptest.NewRequest(check.method, origin+check.path, nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("X-ProtoVoy-Preview", "1")
		if check.authenticated {
			req.SetBasicAuth("fixture", password)
		}
		res := httptest.NewRecorder()
		api.ServeHTTP(res, req)
		if res.Code != check.status {
			t.Errorf("%s %s returned %d, want %d", check.method, check.path, res.Code, check.status)
		}
	}
}

func TestPreviewUsesConfiguredTargetAndRequiresSameOrigin(t *testing.T) {
	var requests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		body, err := io.ReadAll(r.Body)
		if r.Method != http.MethodPost || r.URL.Path != "/configured" || err != nil ||
			string(body) != "{\"name\":\"测试\"}" || len(r.Header.Values("X-Tag")) != 2 {
			t.Error("preview must forward the payload only to the configured endpoint")
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, strings.Repeat("a", execution.PreviewLimit*2))
	}))
	defer target.Close()

	const addr = "127.0.0.1:39300"
	api := handler(loopbackAccess(addr), t.TempDir(), target.URL+"/configured", "")
	for _, tc := range []struct {
		name   string
		host   string
		origin string
		header string
		body   string
		status int
	}{
		{"wrong host", "other.test", "http://" + addr, "1", "", http.StatusForbidden},
		{"foreign origin", addr, "https://other.test", "1", "", http.StatusForbidden},
		{"missing header", addr, "http://" + addr, "", "", http.StatusForbidden},
		{"target override", addr, "http://" + addr, "1", `{"method":"POST","target":"https://other.test"}`, http.StatusBadRequest},
		{"target mismatch", addr, "http://" + addr, "1", `{"method":"POST","expectedTarget":"https://other.test"}`, http.StatusConflict},
		{"multiple documents", addr, "http://" + addr, "1", `{"method":"POST"} {}`, http.StatusBadRequest},
		{"unsupported method", addr, "http://" + addr, "1", `{"method":"CONNECT"}`, http.StatusBadRequest},
		{"redirect override", addr, "http://" + addr, "1", `{"method":"GET","followRedirects":true}`, http.StatusBadRequest},
		{"multipart with separate body", addr, "http://" + addr, "1", `{"method":"POST","body":"ignored","multipart":[{"name":"field","value":"example"}]}`, http.StatusBadRequest},
		{"multipart with Base64 body encoding", addr, "http://" + addr, "1", `{"method":"POST","bodyEncoding":"base64","multipart":[{"name":"field","value":"example"}]}`, http.StatusBadRequest},
		{"oversize body", addr, "http://" + addr, "1", `{"method":"POST","body":"` + strings.Repeat("a", 1<<20) + `"}`, http.StatusRequestEntityTooLarge},
		{"same origin", addr, "http://" + addr, "1", `{"method":"POST","expectedTarget":"` + target.URL + `/configured","headers":[["X-Tag","one"],["X-Tag","two"]],"body":"{\"name\":\"测试\"}"}`, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "http://"+tc.host+"/api/preview?target=https://other.test", strings.NewReader(tc.body))
			request.Header.Set("Origin", tc.origin)
			request.Header.Set("X-ProtoVoy-Preview", tc.header)
			response := httptest.NewRecorder()
			api.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d", response.Code, tc.status)
			}
			if tc.status == http.StatusOK &&
				(response.Header().Get("X-ProtoVoy-Status") != "404" || response.Body.Len() != execution.PreviewLimit+1) {
				t.Fatal("preview should carry the upstream status and the bounded body")
			}
		})
	}
	if requests.Load() != 1 {
		t.Fatalf("configured endpoint received %d requests, want 1", requests.Load())
	}
}
