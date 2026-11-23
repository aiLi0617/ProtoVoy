package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

func TestScheduledDispatcherValidatesBeforeSelectingProtocol(t *testing.T) {
	var httpCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { httpCalls.Add(1); fmt.Fprint(w, "fixture") }))
	defer server.Close()
	target := "sqlite:" + filepath.ToSlash(filepath.Join(t.TempDir(), "queries.db")) + "?mode=rwc"
	database, err := execution.NewSQLEndpoint(target, "", "")
	if err != nil {
		t.Fatal(err)
	}
	additional := map[string]struct{}{server.URL + "/additional": {}}
	execute := scheduledExecutor(server.URL, "", socketTargets{AdditionalHTTP: additional, SQL: &sqlEndpoint{target: target, database: database}})
	delete(additional, server.URL+"/additional") // the running policy must remain stable.
	for _, url := range []string{server.URL, server.URL + "/additional"} {
		if result, err := execute(t.Context(), project.Request{URL: url, Method: "GET"}); err != nil || !result.Success {
			t.Fatalf("HTTP: %+v %v", result, err)
		}
	}
	request := project.Request{URL: target, Body: "CREATE TABLE jobs(id INTEGER)", SQL: &project.SQLSettings{RowLimit: 100}}
	if result, err := execute(t.Context(), request); err != nil || !result.Success {
		t.Fatalf("SQL: %+v %v", result, err)
	}
	before := httpCalls.Load()
	for _, input := range []project.Request{
		{URL: server.URL, Method: "GET", SSE: &execution.SSEOptions{}, WebSocket: &execution.WebSocketOptions{}},
		{URL: server.URL, Method: "GET", Headers: map[string]string{"Authorization": "Bearer secret"}},
		{URL: server.URL, Method: "GET", GRPC: &project.GRPCSettings{Service: "demo.Echo", Method: "Echo", Mode: "unary"}},
	} {
		if result, err := execute(t.Context(), input); err == nil || result.Success {
			t.Fatal("invalid or unconfigured request accepted")
		}
	}
	if httpCalls.Load() != before {
		t.Fatal("invalid or unconfigured protocol fell through to HTTP")
	}
}
