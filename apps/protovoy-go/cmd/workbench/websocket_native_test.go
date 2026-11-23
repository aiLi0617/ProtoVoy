//go:build !js

package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"protovoy.dev/protovoy/internal/execution"
)

func TestWebSocketStartupFramesReplayAndReceiveLimitSpansConnections(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		attempt := attempts.Add(1)
		for _, expected := range []socketFrame{{websocket.MessageText, []byte("hello")}, {websocket.MessageBinary, []byte{0, 255}}} {
			kind, data, err := conn.Read(r.Context())
			if err != nil || kind != expected.kind || !bytes.Equal(data, expected.data) {
				t.Errorf("wrong startup frame: %d, %x, %v", kind, data, err)
				return
			}
		}
		_ = conn.Write(r.Context(), websocket.MessageText, []byte(fmt.Sprintf("reply %d", attempt)))
		if attempt == 1 {
			_ = conn.Close(websocket.StatusNormalClosure, "")
		} else {
			_, _, _ = conn.Read(r.Context())
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	delay, limit := 0, 2
	options := execution.WebSocketAutomation{ReconnectMax: 1, ReconnectDelayMS: &delay, ReceiveLimit: &limit}
	setup := execution.WebSocketSetup{ExpectedTarget: "ws" + strings.TrimPrefix(server.URL, "http")}
	frames := []socketFrame{{websocket.MessageText, []byte("hello")}, {websocket.MessageBinary, []byte{0, 255}}}
	final := readSocket(ctx, connectWebSocket, setup, make(chan socketFrame), options, frames, func(result) {})
	if final.status != "WebSocket completed · 2 received" || attempts.Load() != 2 ||
		!strings.Contains(final.body, "reply 1") || !strings.Contains(final.body, "reply 2") {
		t.Fatalf("unexpected WebSocket result: %q, %d connections, %q", final.status, attempts.Load(), final.body)
	}
}
