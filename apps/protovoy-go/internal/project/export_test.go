package project

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"protovoy.dev/protovoy/internal/execution"
)

func TestPortableProjectRejectsConflictingMultipartBody(t *testing.T) {
	for _, fields := range []string{`"body":"ignored",`, `"bodyEncoding":"base64",`} {
		input := `{"format":"protovoy-multiprotocol-project","version":2,"requests":[{"url":"https://example.test","method":"POST",` +
			fields + `"multipart":[{"name":"field","value":"example"}]}]}`
		if _, err := Read(strings.NewReader(input)); err == nil || !strings.Contains(err.Error(), "multipart requests cannot include") {
			t.Fatalf("expected a multipart conflict, got %v", err)
		}
	}
}

func TestPersistenceRejectsBase64Credentials(t *testing.T) {
	body := base64.StdEncoding.EncodeToString([]byte(`{"password":"fixture-value"}`))
	for _, request := range []Request{
		{URL: "https://example.test", Method: "POST", Body: body, BodyEncoding: "base64"},
		{URL: "https://example.test", Method: "POST", Multipart: []execution.MultipartPart{{Name: "file", Value: body, Base64: true}}},
	} {
		if _, err := Encode(Project{Requests: []Request{request}}); err == nil || !strings.Contains(err.Error(), "credentials") {
			t.Fatalf("expected a credential error, got %v", err)
		}
	}
	draft := Draft{Target: "https://example.test", Body: body, HTTPBodyMode: "base64"}
	if _, err := EncodeWorkspace(Workspace{Draft: draft}); err == nil || !strings.Contains(err.Error(), "credentials") {
		t.Fatalf("expected autosave to reject encoded credentials, got %v", err)
	}
}

func TestEncodePortableProject(t *testing.T) {
	before := Project{Name: "Demo", Requests: []Request{
		{Name: "Create", URL: "{{endpoint}}", Method: "POST", Body: "{\"name\":\"测试\"}",
			Headers:        map[string]string{"Content-Type": "application/json"},
			CollectionPath: []string{"Users", "Write"}, Variables: map[string]string{"endpoint": "https://example.test/users"}},
		{Name: "List", URL: "https://example.test/users", Method: "GET", Headers: map[string]string{}},
	}}
	data, err := Encode(before)
	if err != nil {
		t.Fatal(err)
	}
	// The TypeScript importer accesses headers as an object on every request.
	var doc struct {
		Format   string
		Version  int
		Requests []struct{ Headers map[string]string }
	}
	if err := json.Unmarshal(data, &doc); err != nil || doc.Format != "protovoy-project" ||
		doc.Version != 1 || len(doc.Requests) != 2 || doc.Requests[1].Headers == nil {
		t.Fatalf("invalid portable shape: %v", err)
	}
	after, err := Read(bytes.NewReader(data))
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("round trip changed the project: %v", err)
	}
}

func TestEncodeMultiprotocolProjectRoundTrip(t *testing.T) {
	before := Project{Name: "Mixed", CollectionVariables: map[string]string{"host": "127.0.0.1"}, Requests: []Request{
		{Name: "HTTP", URL: "http://{{host}}/api", Method: "GET"},
		{Name: "GraphQL", URL: "https://example.test/graphql", GraphQL: &execution.GraphQLDocument{Query: "{ viewer { id } }", Variables: "{}"}},
		{Name: "Subscription", URL: "wss://example.test/graphql", GraphQL: &execution.GraphQLDocument{Query: "subscription { events }", Subscription: true}},
		{Name: "SSE", URL: "https://example.test/events", SSE: &execution.SSEOptions{Reconnects: 2}},
		{Name: "WebSocket", URL: "ws://{{host}}/socket", WebSocket: &execution.WebSocketOptions{Subprotocols: "example"}, Body: "hello"},
		{Name: "TCP", URL: "{{host}}:9000", Socket: &execution.SocketPayload{Network: "tcp", Data: "00", Encoding: "hex", Framing: "fixed", FixedLength: 3, TLS: true}},
		{Name: "UDP", URL: "{{host}}:9001", Socket: &execution.SocketPayload{Network: "udp", Data: "00", Encoding: "hex"}},
		{Name: "Redis", Redis: &RedisSettings{Protocol: 3, Database: "2"}, Body: `[["PING"]]`},
		{Name: "JSON-RPC", URL: "http://{{host}}:8080", RPC: &RPCSettings{Protocol: "jsonrpc", Method: "lookup", ID: "9007199254740993"}, Body: "{}"},
		{Name: "SOAP", URL: "http://{{host}}:8080", RPC: &RPCSettings{Protocol: "soap", Version: "1.1", Action: "urn:lookup"}, Body: "<Envelope/>"},
		{Name: "gRPC", URL: "http://{{host}}:50051", GRPC: &GRPCSettings{Service: "demo.Echo", Method: "Watch", Mode: "server_streaming"}, Body: "CAE=", Headers: map[string]string{"x-demo": "fixture"}},
	}}
	data, err := Encode(before)
	if err != nil {
		t.Fatal(err)
	}
	var doc projectFile
	if err := json.Unmarshal(data, &doc); err != nil || doc.Format != multiProjectFormat || doc.Version == nil || *doc.Version != 2 {
		t.Fatalf("unexpected multiprotocol file: %v", err)
	}
	after, err := Read(bytes.NewReader(data))
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("round trip changed the multiprotocol project: %v", err)
	}
}

func TestMultiprotocolProjectRejectsAmbiguousOrUnsupportedSettings(t *testing.T) {
	for _, request := range []string{
		`{"url":"localhost:6379","redis":{"protocol":3,"password":"fixture"},"body":"[[\"PING\"]]"}`,
		`{"url":"localhost:6379","redis":{"protocol":4},"body":"[[\"PING\"]]"}`,
		`{"url":"localhost:6379","redis":{"protocol":3},"body":"[[null]]"}`,
		`{"url":"localhost:6379","redis":{"protocol":3},"socket":{"network":"tcp"},"body":"[[\"PING\"]]"}`,
		`{"url":"localhost:6379","redis":{"protocol":3},"body":"[[\"PING\"]]","preScripts":["fixture"]}`,
	} {
		input := `{"format":"protovoy-multiprotocol-project","version":2,"requests":[` + request + `]}`
		if _, err := Read(strings.NewReader(input)); err == nil {
			t.Fatal("invalid multiprotocol request was accepted")
		}
	}
}

func TestEncodeRefusesPossibleCredentials(t *testing.T) {
	for _, request := range []Request{
		{Headers: map[string]string{"Authorization": "Bearer example"}},
		{URL: "https://example.test/?access_token=example"},
		{URL: "https://user:example@example.test/"},
		{Body: `{"password":"example"}`},
		{Variables: map[string]string{"api_key": "example"}},
		{URL: "localhost:6379", Body: `[["AUTH","example"]]`, Redis: &RedisSettings{Protocol: 2}},
		{URL: "localhost:9000", Socket: &execution.SocketPayload{Network: "tcp", Data: "password=fixture"}},
	} {
		value := Project{Name: "Demo", Requests: []Request{request}}
		if data, err := Encode(value); err == nil || data != nil {
			t.Fatal("sensitive project must not produce export bytes")
		}
	}
	if data, err := Encode(Project{CollectionVariables: map[string]string{"api_key": "fixture"}}); err == nil || data != nil {
		t.Fatal("sensitive collection variables must not produce export bytes")
	}
}
