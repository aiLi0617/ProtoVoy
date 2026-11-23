package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/coder/websocket"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

func TestScheduledWebSocketReconnectsAndChecksReceiveLimit(t *testing.T) {
	for _, reconnect := range []bool{false, true} {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			defer conn.CloseNow()
			kind, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			if kind != websocket.MessageBinary || string(data) != "\x00\xff" {
				t.Error("binary automation frame changed")
			}
			conn.Write(r.Context(), kind, data)
			conn.Close(websocket.StatusNormalClosure, "")
		}))
		target := "ws" + strings.TrimPrefix(server.URL, "http")
		execute := scheduledWebSocketExecutor(target)
		limit, delay := 2, 0
		options := execution.WebSocketAutomation{ReceiveLimit: &limit, ReconnectDelayMS: &delay, Messages: []execution.WebSocketMessage{{Encoding: "binary", Data: "AP8="}}}
		if reconnect {
			options.ReconnectMax = 1
		}
		request := project.Request{URL: "{{target}}", Variables: map[string]string{"target": target}, WebSocket: &execution.WebSocketOptions{WebSocketAutomation: options}}
		wrong := request
		wrong.URL += "/unconfigured"
		if result, err := execute(t.Context(), wrong); err == nil || result.Success {
			t.Fatal("wrong target accepted")
		}
		if calls.Load() != 0 {
			t.Fatal("invalid request reached upstream")
		}
		result, err := execute(t.Context(), request)
		server.Close()
		if reconnect {
			if err != nil || !result.Success || calls.Load() != 2 {
				t.Fatalf("reconnect: %+v %v calls=%d", result, err, calls.Load())
			}
		} else if err == nil || result.Success {
			t.Fatal("premature close counted as success")
		}
	}
}
