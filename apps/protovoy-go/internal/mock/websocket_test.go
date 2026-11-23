//go:build !js

package mock

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestWebSocketMockMessages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.json")
	if err := os.WriteFile(path, []byte(`[{"id":"ws","method":"WS","path":"/events","status":200,"wsMessages":["one","two"]}]`), 0600); err != nil {
		t.Fatal(err)
	}
	app, err := NewHandler(path, "127.0.0.1:39218", "test")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app)
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	client, _, err := websocket.Dial(ctx, strings.Replace(server.URL, "http", "ws", 1)+"/m1/default-project/default/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseNow()
	for _, expected := range []string{"one", "two"} {
		kind, body, err := client.Read(ctx)
		if err != nil || kind != websocket.MessageText || string(body) != expected {
			t.Fatalf("got %v %q %v, want %q", kind, body, err, expected)
		}
	}
	if _, _, err := client.Read(ctx); websocket.CloseStatus(err) != websocket.StatusNormalClosure {
		t.Fatalf("expected normal closure, got %v", err)
	}
}

func TestWebSocketMockCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.json")
	if err := os.WriteFile(path, []byte(`[{"id":"echo","method":"WS","path":"/echo","status":200,"wsEcho":true}]`), 0600); err != nil {
		t.Fatal(err)
	}
	app, err := NewHandler(path, "127.0.0.1:39218", "test")
	if err != nil {
		t.Fatal(err)
	}
	serverContext, stop := context.WithCancel(t.Context())
	defer stop()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		app.ServeHTTP(w, r.WithContext(serverContext))
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	client, _, err := websocket.Dial(ctx, strings.Replace(server.URL, "http", "ws", 1)+"/m1/default-project/default/echo", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseNow()
	payload := []byte{0, 255, 42}
	if err := client.Write(ctx, websocket.MessageBinary, payload); err != nil {
		t.Fatal(err)
	}
	kind, body, err := client.Read(ctx)
	if err != nil || kind != websocket.MessageBinary || !bytes.Equal(body, payload) {
		t.Fatalf("binary echo = %v %v %v", kind, body, err)
	}
	stop()
	if _, _, err := client.Read(ctx); err == nil || ctx.Err() != nil {
		t.Fatalf("expected prompt disconnect after server cancellation, got %v", err)
	}
}
