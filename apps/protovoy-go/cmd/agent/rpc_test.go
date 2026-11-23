package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAgentExecutesLegacyRPCEnvelopes(t *testing.T) {
	for _, fixture := range []struct{ protocol, payload, contentType, action, body string }{
		{"jsonrpc", `{"type":"raw","value":{"method":"lookup","params":{"query":"{{value}}"},"id":9007199254740993}}`,
			"application/json", "", `{"jsonrpc":"2.0","method":"lookup","params":{"query":"a\"b"},"id":9007199254740993}`},
		{"soap", `{"type":"raw","version":"1.1","action":"urn:lookup","envelope":"<Envelope/>"}`,
			"text/xml; charset=utf-8", "urn:lookup", "<Envelope/>"},
		{"soap", `{"type":"raw","value":{"envelope":"<Envelope/>"}}`,
			"application/soap+xml; charset=utf-8", "", "<Envelope/>"},
	} {
		t.Run(fixture.contentType, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil || string(body) != fixture.body || r.Method != http.MethodPost ||
					r.Header.Get("Content-Type") != fixture.contentType || r.Header.Get("SOAPAction") != fixture.action ||
					r.Header.Get("Authorization") != "" {
					t.Error("unexpected upstream RPC exchange")
				}
				_, _ = io.WriteString(w, "fixture reply")
			}))
			defer upstream.Close()
			agent := testAgent(t)
			client := &http.Client{Timeout: 5 * time.Second}
			call := func(method, path, body string) *http.Response {
				t.Helper()
				req, err := http.NewRequest(method, agent.URL+path, strings.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Authorization", "Bearer "+pairingFixture)
				response, err := client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { response.Body.Close() })
				return response
			}
			input := `{"id":"480b7831-08f1-4753-aef3-887ba1324922","protocolId":"` + fixture.protocol +
				`","target":"` + upstream.URL + `","variables":{"value":"a\"b"},"payload":` + fixture.payload + "}"
			start := call(http.MethodPost, "/v1/executions", input)
			var started struct{ ExecutionID string }
			if start.StatusCode != 202 || json.NewDecoder(start.Body).Decode(&started) != nil || !validID(started.ExecutionID) {
				t.Fatalf("could not start RPC: HTTP %d", start.StatusCode)
			}
			stream := call(http.MethodGet, "/v1/executions/"+started.ExecutionID+"/events", "")
			events, err := io.ReadAll(stream.Body)
			if err != nil || stream.StatusCode != 200 || !strings.Contains(string(events), `"preview":"fixture reply"`) ||
				!strings.Contains(string(events), `"protocolId":"`+fixture.protocol+`"`) || !strings.Contains(string(events), `"type":"completed"`) {
				t.Fatalf("RPC execution did not complete: %v, %s", err, events)
			}
		})
	}
}
