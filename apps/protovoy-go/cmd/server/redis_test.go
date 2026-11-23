package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"protovoy.dev/protovoy/internal/execution"
)

func TestRedisBridgeCancelsPendingReadOnShutdown(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	started := make(chan struct{})
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		close(started)
		_, _ = io.Copy(io.Discard, conn)
	}()
	const addr = "127.0.0.1:39300"
	app := handler(loopbackAccess(addr), t.TempDir(), "", "", socketTargets{Redis: listener.Addr().String()})
	request := httptest.NewRequest(http.MethodPost, "http://"+addr+execution.RedisBridgePath, strings.NewReader(`{"commands":[["PING"]]}`))
	request.Header.Set("Origin", "http://"+addr)
	request.Header.Set("X-ProtoVoy-Preview", "1")
	done := make(chan struct{})
	response := httptest.NewRecorder()
	go func() {
		defer close(done)
		app.ServeHTTP(response, request)
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		app.bridge.stop()
		t.Fatal("Redis bridge did not connect")
	}
	app.bridge.stop()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Redis bridge did not stop")
	}
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "execution canceled") {
		t.Fatalf("shutdown returned HTTP %d: %s", response.Code, response.Body.String())
	}
}

func TestValidateHostedRedisTarget(t *testing.T) {
	for _, target := range []string{"redis:0", "redis:65536", "redis:http", "redis://host:6379", "bad host:6379"} {
		if err := validateHostedRedisTarget(target, "0"); err == nil {
			t.Errorf("accepted invalid target %q", target)
		}
	}
	if err := validateHostedRedisTarget("redis:6379", "-1"); err == nil {
		t.Fatal("accepted negative database")
	}
	if err := validateHostedRedisTarget("[::1]:6379", "2"); err != nil {
		t.Fatal(err)
	}
}

func TestLoadRedisCARejectsInvalidPEM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, []byte("not a certificate"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadRedisCA(path); err == nil || !strings.Contains(err.Error(), "no usable certificates") {
		t.Fatalf("expected invalid CA diagnostic, got %v", err)
	}
}

func TestRedisBridgeRequiresSameOriginAndConfiguredEndpoint(t *testing.T) {
	const addr = "127.0.0.1:39300"
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	target := listener.Addr().String()
	wire := make(chan string, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		_ = connection.SetDeadline(time.Now().Add(5 * time.Second))
		data := make([]byte, len("*1\r\n$4\r\nPING\r\n"))
		if _, err := io.ReadFull(connection, data); err == nil {
			wire <- string(data)
			_, _ = io.WriteString(connection, "+PONG\r\n")
		}
	}()
	handler := handler(loopbackAccess(addr), t.TempDir(), "", "", socketTargets{Redis: target})
	for _, test := range []struct {
		name, host, origin, preview, input string
		want                               int
	}{
		{"foreign host", "example.test", "http://" + addr, "1", `{"commands":[["PING"]]}`, http.StatusForbidden},
		{"foreign origin", addr, "https://example.test", "1", `{"commands":[["PING"]]}`, http.StatusForbidden},
		{"missing preview header", addr, "http://" + addr, "", `{"commands":[["PING"]]}`, http.StatusForbidden},
		{"target mismatch", addr, "http://" + addr, "1", `{"expectedTarget":"127.0.0.1:1","commands":[["PING"]]}`, http.StatusConflict},
		{"client cannot change TLS", addr, "http://" + addr, "1", `{"tls":false,"commands":[["PING"]]}`, http.StatusBadRequest},
		{"unknown RESP version", addr, "http://" + addr, "1", `{"protocol":4,"commands":[["PING"]]}`, http.StatusBadRequest},
		{"client cannot supply bridge credentials", addr, "http://" + addr, "1", `{"password":"fixture","commands":[["PING"]]}`, http.StatusBadRequest},
		{"configured target", addr, "http://" + addr, "1", `{"commands":[["PING"]]}`, http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "http://"+test.host+execution.RedisBridgePath, strings.NewReader(test.input))
			request.Header.Set("Origin", test.origin)
			request.Header.Set("X-ProtoVoy-Preview", test.preview)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d", recorder.Code, test.want)
			}
			if test.want == http.StatusOK {
				var values []execution.RedisValue
				if json.Unmarshal(recorder.Body.Bytes(), &values) != nil || len(values) != 1 || values[0].Text != "PONG" {
					t.Fatalf("unexpected Redis response: %s", recorder.Body.String())
				}
				if got := <-wire; got != "*1\r\n$4\r\nPING\r\n" {
					t.Fatalf("unexpected upstream command: %q", got)
				}
			}
		})
	}
}
