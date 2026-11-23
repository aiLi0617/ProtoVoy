//go:build !js

package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"protovoy.dev/protovoy/internal/execution"
)

func TestSSEReconnectsWithLastEventIDAndCancels(t *testing.T) {
	var connections atomic.Int32
	disconnected := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		if connections.Add(1) == 1 {
			fmt.Fprint(w, "retry: 0\nid: 42\ndata: "+strings.Repeat("x", 9000)+"\n\n")
			return
		}
		if r.Header.Get("Last-Event-ID") != "42" {
			t.Error("missing resume ID")
		}
		fmt.Fprint(w, "event: update\ndata: resumed\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(disconnected)
	}))
	defer upstream.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	target, payload, err := execution.PrepareSSE(upstream.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	configuredDelay := 60_000
	final := readSSE(ctx, target, payload, execution.SSEOptions{Reconnects: 1, ReconnectDelayMS: &configuredDelay}, execute, func(update result) {
		if strings.Contains(update.body, "resumed") {
			cancel()
		}
	})
	if final.status != "SSE disconnected" || !strings.Contains(final.body, "update #42\nresumed") ||
		connections.Load() != 2 {
		t.Fatalf("unexpected SSE result: %q, %d connections", final.status, connections.Load())
	}
	select {
	case <-disconnected:
	case <-time.After(time.Second):
		t.Fatal("cancellation did not close the upstream request")
	}
}

func TestSSEUsesConfiguredInitialRetryDelay(t *testing.T) {
	delay := 30
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	var attempts int
	var firstOpen, secondOpen time.Time
	open := func(context.Context, string, execution.HTTPPayload) (*execution.HTTPResponse, error) {
		attempts++
		if attempts == 1 {
			firstOpen = time.Now()
		} else {
			secondOpen = time.Now()
		}
		return &execution.HTTPResponse{
			StatusCode: http.StatusOK, Headers: http.Header{"Content-Type": {"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader("data: fixture\n\n")),
		}, nil
	}
	final := readSSE(ctx, "", execution.HTTPPayload{}, execution.SSEOptions{Reconnects: 1, ReconnectDelayMS: &delay}, open, func(result) {})
	if attempts != 2 || secondOpen.Sub(firstOpen) < time.Duration(delay)*time.Millisecond || final.status != "SSE closed · 2 events" {
		t.Fatalf("initial SSE retry delay was ignored: %d attempts, %s, %s", attempts, secondOpen.Sub(firstOpen), final.status)
	}
}
