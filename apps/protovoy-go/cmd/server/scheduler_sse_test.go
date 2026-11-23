package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

func TestScheduledSSEReconnectsAndHonorsDeadline(t *testing.T) {
	for _, scenario := range []string{"reconnect", "timeout", "wrong content type"} {
		t.Run(scenario, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempt := calls.Add(1)
				if scenario == "wrong content type" {
					fmt.Fprint(w, "not an event stream")
					return
				}
				if attempt == 2 && r.Header.Get("Last-Event-ID") != "fixture" {
					t.Error("reconnection lost last event ID")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "id: fixture\ndata: event\n\n")
				if scenario == "timeout" {
					w.(http.Flusher).Flush()
					<-r.Context().Done()
				}
			}))
			defer server.Close()
			delay := 0
			request := project.Request{URL: "{{target}}", Variables: map[string]string{"target": server.URL}, HTTPTimeoutMS: 200, SSE: &execution.SSEOptions{Reconnects: 1, ReconnectDelayMS: &delay}}
			execute := scheduledSSEExecutor(server.URL, nil)
			wrong := request
			wrong.URL += "/unconfigured"
			if result, err := execute(t.Context(), wrong); err == nil || result.Success {
				t.Fatal("unconfigured target accepted")
			}
			if calls.Load() != 0 {
				t.Fatal("invalid request reached upstream")
			}
			result, err := execute(t.Context(), request)
			if scenario == "reconnect" {
				if err != nil || !result.Success || calls.Load() != 2 {
					t.Fatalf("reconnect: %+v %v calls=%d", result, err, calls.Load())
				}
			} else if err == nil || result.Success {
				t.Fatalf("failure accepted: %+v %v", result, err)
			}
		})
	}
}
