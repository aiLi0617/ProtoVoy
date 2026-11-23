package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"protovoy.dev/protovoy/internal/agentclient"
	"protovoy.dev/protovoy/internal/execution"
)

func TestAgentWebSocketTicketBindsOriginAndIsSingleUse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test") != "fixture" || r.Header.Get("Authorization") != "" {
			t.Error("upstream received incorrect setup headers")
		}
		connection, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"fixture"}})
		if err != nil {
			return
		}
		defer connection.CloseNow()
		kind, data, err := connection.Read(r.Context())
		if err == nil {
			if err := connection.Write(r.Context(), kind, data); err == nil {
				_ = connection.Close(websocket.StatusNormalClosure, "")
			}
		}
	}))
	defer upstream.Close()
	agent := testAgentWithResponseLimit(t, 16)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	requestID := agentclient.NewRequestID()
	body, err := json.Marshal(struct {
		execution.WebSocketSetup
		RequestID string `json:"requestId"`
	}{
		WebSocketSetup: execution.WebSocketSetup{
			ExpectedTarget: "ws" + strings.TrimPrefix(upstream.URL, "http"),
			Headers:        [][2]string{{"X-Test", "fixture"}},
			Subprotocols:   []string{"fixture"},
		},
		RequestID: requestID,
	})
	if err != nil {
		t.Fatal(err)
	}
	ticketRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, agent.URL+execution.AgentWebSocketTicketPath, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	ticketRequest.Header.Set("Origin", browserOrigin)
	rejected, err := agent.Client().Do(ticketRequest.Clone(ctx))
	if err != nil {
		t.Fatal(err)
	}
	rejected.Body.Close()
	if rejected.StatusCode != http.StatusUnauthorized {
		t.Fatal("anonymous request could mint a WebSocket ticket")
	}
	ticketRequest.Body, err = ticketRequest.GetBody()
	if err != nil {
		t.Fatal(err)
	}
	ticketRequest.Header.Set("Authorization", "Bearer "+pairingFixture)
	response, err := agent.Client().Do(ticketRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var issued struct{ Ticket string }
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&issued) != nil || len(issued.Ticket) != 64 {
		t.Fatal("Agent did not issue a WebSocket ticket")
	}
	options := &websocket.DialOptions{Subprotocols: []string{
		execution.AgentWebSocketProtocol, execution.AgentWebSocketTicketPrefix + issued.Ticket,
	}}
	endpoint := "ws" + strings.TrimPrefix(agent.URL, "http") + execution.AgentWebSocketPath
	if _, rejected, err := websocket.Dial(ctx, endpoint, options); err == nil || rejected == nil || rejected.StatusCode != http.StatusUnauthorized {
		t.Fatal("ticket was accepted without its issuing origin")
	}
	options.HTTPHeader = http.Header{"Origin": {browserOrigin}}
	connection, _, err := websocket.Dial(ctx, endpoint, options)
	if err != nil {
		t.Fatal("could not upgrade with a valid ticket")
	}
	defer connection.CloseNow()
	if connection.Subprotocol() != execution.AgentWebSocketProtocol {
		t.Fatal("bridge did not negotiate its public protocol")
	}
	var ready execution.WebSocketReady
	if wsjson.Read(ctx, connection, &ready) != nil || ready.Subprotocol != "fixture" || ready.Error != "" {
		t.Fatal("upstream did not negotiate its subprotocol")
	}
	want := []byte{0, 1, 255}
	if err := connection.Write(ctx, websocket.MessageBinary, want); err != nil {
		t.Fatal(err)
	}
	kind, received, err := connection.Read(ctx)
	if err != nil || kind != websocket.MessageBinary || !bytes.Equal(received, want) {
		t.Fatal("Agent relay changed the binary frame")
	}
	if _, rejected, err := websocket.Dial(ctx, endpoint, options); err == nil || rejected == nil || rejected.StatusCode != http.StatusUnauthorized {
		t.Fatal("WebSocket ticket was accepted twice")
	}

	history, err := agentclient.Pair(ctx, agent.Listener.Addr().String(), pairingFixture, agent.Client().Do)
	if err != nil {
		t.Fatal(err)
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		items, err := history.History(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 0 {
			record := items[0]
			if len(items) != 1 || record.ProtocolID != "websocket" || record.State != "completed" || record.RequestID != requestID ||
				record.BytesReceived != int64(len(want)) || record.ResponseBodyBytes != nil {
				t.Fatalf("unexpected WebSocket history: %+v", items)
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("WebSocket history did not record the closed connection")
		case <-ticker.C:
		}
	}
}
