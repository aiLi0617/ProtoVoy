package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

func TestScheduledGraphQLSubscriptionOutcomes(t *testing.T) {
	for _, scenario := range []string{"success", "rejected", "data errors", "timeout"} {
		t.Run(scenario, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{execution.GraphQLWebSocketProtocol}})
				if err != nil {
					return
				}
				defer conn.CloseNow()
				ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
				defer cancel()
				var message map[string]any
				if wsjson.Read(ctx, conn, &message) != nil || message["type"] != "connection_init" {
					return
				}
				wsjson.Write(ctx, conn, map[string]any{"type": "connection_ack"})
				if wsjson.Read(ctx, conn, &message) != nil || message["type"] != "subscribe" {
					return
				}
				id := message["id"]
				if scenario == "timeout" {
					wsjson.Read(ctx, conn, &message)
					return
				}
				if scenario == "rejected" {
					wsjson.Write(ctx, conn, map[string]any{"type": "error", "id": id, "payload": []any{map[string]any{"message": "secret diagnostic"}}})
					return
				}
				payload := map[string]any{"data": map[string]any{"value": 42}}
				if scenario == "data errors" {
					payload["errors"] = []any{map[string]any{"message": "secret diagnostic"}}
				}
				wsjson.Write(ctx, conn, map[string]any{"type": "next", "id": id, "payload": payload})
				wsjson.Write(ctx, conn, map[string]any{"type": "complete", "id": id})
			}))
			defer server.Close()
			target := "ws" + strings.TrimPrefix(server.URL, "http")
			execute := scheduledGraphQLExecutor(target)
			request := project.Request{URL: "{{target}}", Variables: map[string]string{"target": target}, GraphQL: &execution.GraphQLDocument{Subscription: true, Query: "subscription { value }"}}
			wrong := request
			wrong.URL += "/unconfigured"
			if result, err := execute(t.Context(), wrong); err == nil || result.Success {
				t.Fatal("wrong target accepted")
			}
			if calls.Load() != 0 {
				t.Fatal("unconfigured request reached upstream")
			}
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			result, err := execute(ctx, request)
			if scenario == "success" {
				if err != nil || !result.Success {
					t.Fatalf("subscription failed: %+v %v", result, err)
				}
			} else if err == nil || result.Success {
				t.Fatalf("failure counted as success: %+v %v", result, err)
			}
		})
	}
}
