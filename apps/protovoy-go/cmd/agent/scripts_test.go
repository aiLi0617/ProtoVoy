package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAgentScriptedHTTPThroughAuthenticatedExecution(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jobs" || r.Header.Get("X-Step") != "ready" {
			t.Error("pre-request script changes were missing")
		}
		time.Sleep(40 * time.Millisecond)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("script-response-fixture"))
	}))
	defer upstream.Close()
	agent := testAgent(t)
	client := &http.Client{Timeout: 12 * time.Second}
	call := func(method, path string, body any) *http.Response {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(t.Context(), method, agent.URL+path, bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+pairingFixture)
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	for _, pass := range []bool{true, false} {
		post := "assert(false);"
		if pass {
			post = "assert(response.status === 201 && response.body === 'script-response-fixture' && variables.step === 'ready' && response.durationMs >= 35);"
		}
		envelope := map[string]any{
			"id": "480b7831-08f1-4753-aef3-887ba1324922", "protocolId": "http", "target": upstream.URL,
			"payload":     map[string]any{"type": "http", "method": "GET", "headers": [][2]string{}},
			"preScripts":  []string{`request.target += '/jobs'; variables.step = 'ready'; request.payload.headers.push(['X-Step', variables.step]);`},
			"postScripts": []string{post},
		}
		start := call("POST", "/v1/executions", envelope)
		var execution struct{ ExecutionID string }
		err := json.NewDecoder(start.Body).Decode(&execution)
		start.Body.Close()
		if err != nil || start.StatusCode != 202 {
			t.Fatalf("script execution rejected: %d, %v", start.StatusCode, err)
		}
		stream := call("GET", "/v1/executions/"+execution.ExecutionID+"/events", nil)
		scanner := bufio.NewScanner(stream.Body)
		var completed, failed, body bool
		for scanner.Scan() {
			event := readTestEvent(t, scanner.Text())
			switch event["type"] {
			case "completed":
				completed = true
			case "failed":
				failed = event["code"] == "post-response script"
			case "response_chunk":
				body = body || event["size"] != float64(0)
			}
		}
		stream.Body.Close()
		if scanner.Err() != nil || completed != pass || failed == pass || body != pass {
			t.Fatalf("unexpected script event sequence: completed=%v failed=%v body=%v err=%v", completed, failed, body, scanner.Err())
		}
	}
}
