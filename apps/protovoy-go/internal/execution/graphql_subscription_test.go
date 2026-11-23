//go:build !js

package execution

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestSubscriptionAcknowledgesPingAndCompletesOnCancel(t *testing.T) {
	completed := make(chan bool, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{GraphQLWebSocketProtocol}})
		if err != nil {
			return
		}
		defer connection.CloseNow()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var message graphQLSocketMessage
		if wsjson.Read(ctx, connection, &message) != nil || message.Type != "connection_init" {
			return
		}
		_ = wsjson.Write(ctx, connection, graphQLSocketMessage{Type: "ping", Payload: json.RawMessage(`{"nonce":1}`)})
		if wsjson.Read(ctx, connection, &message) != nil || message.Type != "pong" || string(message.Payload) != `{"nonce":1}` {
			return
		}
		_ = wsjson.Write(ctx, connection, graphQLSocketMessage{Type: "connection_ack"})
		if wsjson.Read(ctx, connection, &message) != nil || message.Type != "subscribe" || message.ID != "1" {
			return
		}
		_ = wsjson.Write(ctx, connection, graphQLSocketMessage{Type: "next", ID: "1", Payload: json.RawMessage(`{"data":{"value":1}}`)})
		if wsjson.Read(ctx, connection, &message) == nil {
			completed <- message.Type == "complete" && message.ID == "1"
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, err := DialWebSocket(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), WebSocketSetup{Subprotocols: []string{GraphQLWebSocketProtocol}})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.CloseNow()
	ready, results := false, 0
	err = RunGraphQLSubscription(ctx, connection, json.RawMessage(`{"query":"subscription { value }"}`), func() { ready = true }, func(data json.RawMessage, rejected bool) {
		if string(data) != `{"data":{"value":1}}` || rejected {
			t.Error("unexpected subscription result")
		}
		results++
		cancel()
	})
	if err != context.Canceled || !ready || results != 1 {
		t.Fatalf("subscription outcome: %v, %v, %d", err, ready, results)
	}
	select {
	case ok := <-completed:
		if !ok {
			t.Fatal("missing complete message")
		}
	case <-time.After(time.Second):
		t.Fatal("server did not receive complete")
	}
}
