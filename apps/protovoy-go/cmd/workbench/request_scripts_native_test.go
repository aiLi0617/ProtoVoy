//go:build !js

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vivaui/viva/text"
)

func TestDesktopSendExecutesRequestScripts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jobs" || r.Header.Get("X-Script") != "ready" {
			t.Error("desktop pre-script changes did not reach HTTP server")
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	}))
	defer server.Close()
	w := newWorkbench()
	w.target.SetText(server.URL)
	w.preScripts.input.Editor = text.NewEditor(formatRequestScripts([]string{
		`request.target += '/jobs'; variables.step = 'ready'; request.payload.headers.push(['X-Script', variables.step]);`,
	}))
	w.postScripts.input.Editor = text.NewEditor(formatRequestScripts([]string{
		`assert(response.status === 201 && response.body === 'created' && variables.step === 'ready');`,
	}))
	w.send()
	if w.cancel == nil {
		t.Fatalf("desktop request did not start: %s", w.result.status)
	}
	defer w.cancel()
	pending := awaitDesktopScript(t, w)
	if pending.status != "HTTP 201" || pending.body != "created" {
		t.Fatalf("unexpected desktop result: %s", pending.status)
	}
}

func awaitDesktopScript(t *testing.T, w *workbench) result {
	t.Helper()
	timer := time.NewTimer(12 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-timer.C:
			t.Fatal("desktop scripted request did not finish")
		case <-ticker.C:
			w.mu.Lock()
			pending := w.pending
			w.mu.Unlock()
			if pending != nil && pending.done {
				return *pending
			}
		}
	}
}

func TestDesktopScriptFailuresAndCancellation(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("private-response-fixture"))
	}))
	defer server.Close()
	for _, test := range []struct {
		name, pre, post, status string
		cancel                  bool
		expectedRequests        int32
	}{
		{name: "pre assertion", pre: "assert(false);", status: "pre-request script", expectedRequests: 0},
		{name: "post assertion", post: "assert(false);", status: "HTTP 201 · post-response script", expectedRequests: 1},
		{name: "cancel", pre: "while (true) {}", status: "Cancelled", cancel: true, expectedRequests: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := requests.Load()
			w := newWorkbench()
			w.target.SetText(server.URL)
			if test.pre != "" {
				w.preScripts.input.Editor = text.NewEditor(formatRequestScripts([]string{test.pre}))
			}
			if test.post != "" {
				w.postScripts.input.Editor = text.NewEditor(formatRequestScripts([]string{test.post}))
			}
			w.send()
			if w.cancel == nil {
				t.Fatalf("request did not start: %s", w.result.status)
			}
			defer w.cancel()
			if test.cancel {
				w.cancel()
			}
			result := awaitDesktopScript(t, w)
			if !strings.Contains(result.status, test.status) || result.body != "" || len(result.rawPreview) != 0 {
				t.Fatalf("unexpected failed script result: %s", result.status)
			}
			if requests.Load()-before != test.expectedRequests {
				t.Fatal("script failure did not stop the expected HTTP phase")
			}
		})
	}
}
