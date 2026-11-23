package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"protovoy.dev/protovoy/internal/execution"
)

func TestGraphQLCLIHTTP(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			OperationName string
			Variables     map[string]int
		}
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("X-Trace") != "fixture" ||
			json.NewDecoder(r.Body).Decode(&request) != nil || request.OperationName != "Change" || request.Variables["id"] != 7 {
			t.Error("unexpected GraphQL HTTP request")
		}
		_, _ = w.Write([]byte(`{"data":{"change":true}}`))
	}))
	defer upstream.Close()
	var output, status bytes.Buffer
	err := runGraphQL(context.Background(), upstream.URL, execution.GraphQLDocument{
		Query: `mutation Change($id: Int!) { change(id: $id) }`, OperationName: "Change", Variables: `{"id":7}`,
	}, [][2]string{{"X-Trace", "fixture"}}, 5*time.Second, &output, &status)
	if err != nil || output.String() != `{"data":{"change":true}}` || status.String() != "HTTP 200\n" {
		t.Fatalf("unexpected command result: %q, %q, %v", output.String(), status.String(), err)
	}
}

func TestGraphQLCLISubscriptionStreamsJSONLines(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Trace") != "fixture" {
			t.Error("subscription handshake lost the request header")
		}
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{execution.GraphQLWebSocketProtocol}})
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.CloseNow()
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		var message struct{ Type, ID string }
		if err := wsjson.Read(ctx, conn, &message); err != nil || message.Type != "connection_init" {
			t.Errorf("invalid init: %+v, %v", message, err)
			return
		}
		if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"connection_ack"}`)); err != nil {
			t.Error(err)
			return
		}
		if err := wsjson.Read(ctx, conn, &message); err != nil || message.Type != "subscribe" || message.ID != "1" {
			t.Errorf("invalid subscribe: %+v, %v", message, err)
			return
		}
		for _, frame := range []string{
			`{"type":"next","id":"1","payload":{"data":{"updates":1}}}`,
			`{"type":"next","id":"1","payload":{"data":{"updates":2}}}`,
			`{"type":"complete","id":"1"}`,
		} {
			if err := conn.Write(ctx, websocket.MessageText, []byte(frame)); err != nil {
				t.Error(err)
				return
			}
		}
	}))
	defer upstream.Close()
	var output bytes.Buffer
	err := runGraphQL(context.Background(), upstream.URL, execution.GraphQLDocument{
		Query: "subscription { updates }", Subscription: true,
	}, [][2]string{{"X-Trace", "fixture"}}, 5*time.Second, &output, &bytes.Buffer{})
	if err != nil || output.String() != "{\"data\":{\"updates\":1}}\n{\"data\":{\"updates\":2}}\n" {
		t.Fatalf("unexpected subscription result: %q, %v", output.String(), err)
	}
}
