package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const pairingFixture = "pairing-token-used-in-tests-only"
const browserOrigin = "https://workbench.example"

func TestAgentPairingAndHTTPEventStream(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("pairing authorization was sent upstream")
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte{0, 1, 255})
	}))
	defer target.Close()
	agent := testAgent(t)
	client := &http.Client{Timeout: 5 * time.Second}
	call := func(method, path, origin, token, version string) *http.Response {
		t.Helper()
		body, _ := json.Marshal(map[string]any{
			"id": "480b7831-08f1-4753-aef3-887ba1324922", "protocolId": "http",
			"target": target.URL, "payload": map[string]any{"type": "http", "method": "GET"},
		})
		request, err := http.NewRequest(method, agent.URL+path, strings.NewReader(string(body)))
		if err != nil {
			t.Fatal(err)
		}
		if origin != "" {
			request.Header.Set("Origin", origin)
		}
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		if version != "" {
			request.Header.Set(versionHeader, version)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { response.Body.Close() })
		return response
	}
	expectStatus := func(response *http.Response, want int) {
		t.Helper()
		if response.StatusCode != want {
			t.Fatalf("status = %d, want %d", response.StatusCode, want)
		}
	}
	expectStatus(call("GET", "/health", "", "", ""), 200)
	expectStatus(call("POST", "/v1/session", "https://foreign.example", pairingFixture, apiVersion), 403)
	expectStatus(call("POST", "/v1/session", browserOrigin, "", apiVersion), 401)
	expectStatus(call("POST", "/v1/session", browserOrigin, pairingFixture, "2"), 426)
	preflight := call("OPTIONS", "/v1/executions", browserOrigin, "", "")
	expectStatus(preflight, 204)
	if preflight.Header.Get("Access-Control-Allow-Origin") != browserOrigin {
		t.Fatal("allowed preflight did not advertise the exact origin")
	}
	paired := call("POST", "/v1/session", browserOrigin, pairingFixture, apiVersion)
	expectStatus(paired, 200)
	var session struct{ Token string }
	if json.NewDecoder(paired.Body).Decode(&session) != nil || len(session.Token) != 64 {
		t.Fatal("expected a session token")
	}
	expectStatus(call("POST", "/v1/executions", "", session.Token, apiVersion), 401)
	started := call("POST", "/v1/executions", browserOrigin, session.Token, apiVersion)
	expectStatus(started, 202)
	var execution struct{ ExecutionID string }
	if json.NewDecoder(started.Body).Decode(&execution) != nil || !validID(execution.ExecutionID) {
		t.Fatal("expected an execution ID")
	}
	path := "/v1/executions/" + execution.ExecutionID + "/events"
	expectStatus(call("HEAD", path, browserOrigin, session.Token, apiVersion), 405)
	stream := call("GET", path, browserOrigin, session.Token, apiVersion)
	expectStatus(stream, 200)
	var gotBytes, gotStatus, completed bool
	reader := bufio.NewScanner(stream.Body)
	for reader.Scan() {
		event := readTestEvent(t, reader.Text())
		switch event["type"] {
		case "response_meta":
			gotStatus = event["status"] == float64(201)
		case "response_chunk":
			gotBytes = gotBytes || event["data_base64"] == base64.StdEncoding.EncodeToString([]byte{0, 1, 255})
		case "completed":
			completed = event["summary"].(map[string]any)["bytesReceived"] == float64(3)
		}
	}
	if reader.Err() != nil || !gotBytes || !gotStatus || !completed {
		t.Fatalf("missing response metadata, exact binary bytes or completion: %v", reader.Err())
	}
	expectStatus(call("GET", path, browserOrigin, session.Token, apiVersion), 404)
	expectStatus(call("GET", "/v1/history", browserOrigin, "", apiVersion), 401)
	expectStatus(call("GET", "/v1/history/"+execution.ExecutionID+"/body", browserOrigin, session.Token, apiVersion), 404)
	history := call("GET", "/v1/history?protocolId=http&state=completed&status=201", browserOrigin, session.Token, apiVersion)
	expectStatus(history, 200)
	var records []struct {
		ID            string
		BytesReceived int64
	}
	if json.NewDecoder(history.Body).Decode(&records) != nil || len(records) != 1 ||
		records[0].ID != execution.ExecutionID || records[0].BytesReceived != 3 {
		t.Fatal("completed execution is missing from filtered history")
	}
}

func TestAgentCancelsInFlightHTTP(t *testing.T) {
	cancelled := make(chan struct{})
	testDone := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "first chunk")
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
			close(cancelled)
		case <-testDone:
		}
	}))
	defer upstream.Close()
	defer close(testDone)
	agent := testAgent(t)
	client := &http.Client{Timeout: 5 * time.Second}
	call := func(method, path, body string) *http.Response {
		t.Helper()
		r, err := http.NewRequest(method, agent.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", "Bearer "+pairingFixture)
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { response.Body.Close() })
		return response
	}
	body := `{"id":"480b7831-08f1-4753-aef3-887ba1324922","protocolId":"http","target":"` + upstream.URL + `","payload":{"type":"http","method":"GET"}}`
	started := call("POST", "/v1/executions", body)
	var response struct{ ExecutionID string }
	if started.StatusCode != 202 || json.NewDecoder(started.Body).Decode(&response) != nil || !validID(response.ExecutionID) {
		t.Fatal("could not start fixture execution")
	}
	path := "/v1/executions/" + response.ExecutionID
	stream := call("GET", path+"/events", "")
	reader := bufio.NewScanner(stream.Body)
	for reader.Scan() {
		if readTestEvent(t, reader.Text())["type"] == "response_chunk" {
			break
		}
	}
	if reader.Err() != nil {
		t.Fatal(reader.Err())
	}
	result := call("POST", path+"/cancel", "")
	var outcome struct{ Cancelled bool }
	if result.StatusCode != 200 || json.NewDecoder(result.Body).Decode(&outcome) != nil || !outcome.Cancelled {
		t.Fatal("could not cancel active execution")
	}
	found := false
	for reader.Scan() {
		found = found || readTestEvent(t, reader.Text())["type"] == "cancelled"
	}
	if reader.Err() != nil || !found {
		t.Fatal("missing terminal cancellation event")
	}
	history := call("GET", "/v1/history/"+response.ExecutionID, "")
	var record struct{ State string }
	if history.StatusCode != 200 || json.NewDecoder(history.Body).Decode(&record) != nil || record.State != "cancelled" {
		t.Fatal("cancelled execution is missing from history")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("upstream did not see cancellation")
	}
}

func TestAgentSSEReconnectsWithLastEventID(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Error("upstream SSE request did not advertise the event stream")
		}
		if calls.Add(1) == 1 {
			if r.Header.Get("Last-Event-ID") != "initial" {
				t.Error("initial event ID was missing")
			}
			_, _ = io.WriteString(w, "id: 42\nretry: 1\ndata: first\n\n")
			return
		}
		if r.Header.Get("Last-Event-ID") != "42" {
			t.Error("reconnect did not use the latest event ID")
		}
		_, _ = io.WriteString(w, "data: resumed\n\n")
	}))
	defer upstream.Close()
	agent := testAgent(t)
	client := &http.Client{Timeout: 5 * time.Second}
	call := func(method, path, body string) *http.Response {
		t.Helper()
		request, err := http.NewRequest(method, agent.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+pairingFixture)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { response.Body.Close() })
		return response
	}
	input := `{"id":"480b7831-08f1-4753-aef3-887ba1324922","protocolId":"sse","target":"` + upstream.URL + `","payload":{"type":"sse","lastEventId":"initial","reconnectMax":1,"reconnectDelayMs":60000}}`
	started := call("POST", "/v1/executions", input)
	var response struct{ ExecutionID string }
	if started.StatusCode != 202 || json.NewDecoder(started.Body).Decode(&response) != nil || !validID(response.ExecutionID) {
		t.Fatalf("could not start fixture SSE execution: HTTP %d", started.StatusCode)
	}
	stream := call("GET", "/v1/executions/"+response.ExecutionID+"/events", "")
	if stream.StatusCode != 200 {
		t.Fatalf("could not read fixture events: HTTP %d", stream.StatusCode)
	}
	var chunks strings.Builder
	var warning, completed bool
	reader := bufio.NewScanner(stream.Body)
	for reader.Scan() {
		event := readTestEvent(t, reader.Text())
		switch event["type"] {
		case "response_chunk":
			if data, ok := event["data_base64"].(string); ok {
				bytes, err := base64.StdEncoding.DecodeString(data)
				if err != nil {
					t.Fatal(err)
				}
				chunks.Write(bytes)
			}
		case "warning":
			warning = event["code"] == "sse_reconnect"
		case "completed":
			completed = event["summary"].(map[string]any)["protocolId"] == "sse"
		}
	}
	if reader.Err() != nil || !warning || !completed || calls.Load() != 2 ||
		chunks.String() != "id: 42\nretry: 1\ndata: first\n\ndata: resumed\n\n" {
		t.Fatalf("SSE reconnect lost raw bytes or terminal events: %v", reader.Err())
	}
}

func testAgent(t *testing.T) *httptest.Server {
	t.Helper()
	return testAgentWithResponseLimit(t, 0)
}

func testAgentWithResponseLimit(t *testing.T, limit int) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(nil)
	history, err := openHistory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { history.root.Close() })
	history.responseLimit = limit
	agent, err := newAgent(server.Listener.Addr().String(), pairingFixture, browserOrigin, history)
	if err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = agent.routes()
	server.Start()
	t.Cleanup(server.Close)
	return server
}

func readTestEvent(t *testing.T, line string) agentEvent {
	t.Helper()
	data, ok := strings.CutPrefix(line, "data: ")
	if !ok {
		return nil
	}
	var event agentEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		t.Fatal(err)
	}
	return event
}
