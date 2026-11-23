package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

func TestCollectionWebSocketResendsAndCountsAcrossReconnects(t *testing.T) {
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"echo"}})
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.CloseNow()
		connections.Add(1)
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		kind, data, err := conn.Read(ctx)
		if err != nil || kind != websocket.MessageText || string(data) != "hello" || conn.Subprotocol() != "echo" {
			t.Errorf("unexpected startup frame: %v %q (%v)", kind, data, err)
			return
		}
		if err := conn.Write(ctx, websocket.MessageText, []byte("received")); err != nil {
			t.Error(err)
			return
		}
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}))
	defer server.Close()
	limit, delay := 2, 0
	var output bytes.Buffer
	request := project.Request{
		Name: "reconnect", URL: "ws" + strings.TrimPrefix(server.URL, "http"),
		WebSocket: &execution.WebSocketOptions{
			Subprotocols: "echo",
			WebSocketAutomation: execution.WebSocketAutomation{
				Messages: []execution.WebSocketMessage{{Data: "{{message}}"}}, ReceiveLimit: &limit,
				ReconnectMax: 1, ReconnectDelayMS: &delay,
			},
		},
	}
	err := runCollection(context.Background(), project.Project{
		Requests: []project.Request{request}, CollectionVariables: map[string]string{"message": "hello"},
	}, collectionRunOptions{streamTimeout: 3 * time.Second}, &output)
	var result collectionResult
	if decodeErr := json.NewDecoder(&output).Decode(&result); err != nil || decodeErr != nil || !result.Passed ||
		result.Protocol != "websocket" || connections.Load() != 2 {
		t.Fatalf("unexpected collection outcome: %+v, %v, %v, %d connections", result, err, decodeErr, connections.Load())
	}
}
