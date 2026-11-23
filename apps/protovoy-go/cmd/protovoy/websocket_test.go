package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"protovoy.dev/protovoy/internal/execution"
)

func TestWebSocketCLIExchangesBinaryAndText(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Trace") != "fixture" || r.Header.Get("Sec-WebSocket-Protocol") != "echo" {
			t.Error("WebSocket handshake lost the configured header or subprotocol")
		}
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"echo"}})
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.CloseNow()
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		kind, data, err := conn.Read(ctx)
		if err != nil || kind != websocket.MessageBinary || !bytes.Equal(data, []byte{0, 255}) {
			t.Errorf("unexpected startup frame: %d, %v, %v", kind, data, err)
			return
		}
		if err := conn.Write(ctx, websocket.MessageText, []byte("ready")); err != nil {
			t.Error(err)
			return
		}
		if err := conn.Write(ctx, websocket.MessageBinary, data); err != nil {
			t.Error(err)
		}
		// The CLI must return at the receive limit while the peer is still open.
		_, _, _ = conn.Read(ctx)
	}))
	defer upstream.Close()
	limit := 2
	options := execution.WebSocketAutomation{ReceiveLimit: &limit, Messages: []execution.WebSocketMessage{
		{Encoding: "binary", Data: "AP8="},
	}}
	var output bytes.Buffer
	err := runWebSocket(context.Background(), "ws"+strings.TrimPrefix(upstream.URL, "http"),
		[][2]string{{"X-Trace", "fixture"}}, "echo", options, 5*time.Second, &output)
	want := "{\"encoding\":\"text\",\"data\":\"ready\"}\n{\"encoding\":\"binary\",\"data\":\"AP8=\"}\n"
	if err != nil || output.String() != want {
		t.Fatalf("unexpected WebSocket output: %q, %v", output.String(), err)
	}
}
