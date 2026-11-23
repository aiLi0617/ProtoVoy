package project

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestGraphQLHTTPTimeoutRoundTripsInBothFormats(t *testing.T) {
	input := `{"protocolId":"graphql","target":"http://localhost/graphql","timeoutMs":450,"payload":{"type":"graphql","query":"query { __typename }"}}`
	value, err := Read(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	request := value.Requests[0]
	_, payload, err := request.GraphQL.Prepare(request.URL, nil)
	if err != nil || payload.TimeoutMS != 450 {
		t.Fatalf("GraphQL wire request lost its timeout: %v", err)
	}
	standalone, err := EncodeGraphQL(request.Name, request.URL, nil, *request.GraphQL, nil)
	if err != nil {
		t.Fatal(err)
	}
	portable, err := Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{standalone, portable} {
		restored, err := Read(bytes.NewReader(data))
		if err != nil || restored.Requests[0].GraphQL.TimeoutMS != 450 {
			t.Fatalf("GraphQL timeout was not preserved: %v", err)
		}
	}
	subscription := strings.Replace(input, "query { __typename }", "subscription { updates }", 1)
	if _, err := Read(strings.NewReader(subscription)); err == nil {
		t.Fatal("accepted a custom HTTP timeout on a WebSocket subscription")
	}
}

func TestGraphQLImportChecksLegacyExecutionSettings(t *testing.T) {
	const input = `{"protocolId":"graphql","target":"https://localhost/graphql","payload":{"type":"graphql","query":"query { __typename }"}`
	compatible := input + `,"authRef":null,"metadata":{},"tls":{"verify":true,"client_cert_ref":null},"retryPolicy":{"max_retries":0,"backoff_ms":0}}`
	if _, err := Read(strings.NewReader(compatible)); err != nil {
		t.Fatalf("could not import GraphQL with default legacy settings: %v", err)
	}
	for _, setting := range []string{
		`"tls":{"verify":false}`,
		`"tls":{"verify":true,"client_cert_ref":"private-ca"}`,
		`"retryPolicy":{"max_retries":2,"backoff_ms":100}`,
		`"metadata":{"customTransport":true}`,
	} {
		if _, err := Read(strings.NewReader(input + "," + setting + "}")); err == nil {
			t.Errorf("import silently discarded %s", setting)
		}
	}
}

func TestGraphQLRequestFileRoundTrip(t *testing.T) {
	file, err := os.Open("../../../../examples/graphql/typename.query.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	value, err := Read(file)
	if err != nil {
		t.Fatal(err)
	}
	request := value.Requests[0]
	if request.GraphQL == nil || request.GraphQL.OperationName != "Example" ||
		!strings.Contains(request.GraphQL.Query, "__typename") {
		t.Fatalf("GraphQL example imported incorrectly: %+v", request)
	}
	data, err := EncodeGraphQL(request.Name, request.URL, [][2]string{{"X-Test", "fixture"}}, *request.GraphQL, nil)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Read(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Requests[0].Headers["X-Test"] != "fixture" || *reopened.Requests[0].GraphQL != *request.GraphQL {
		t.Fatal("GraphQL file lost request fields")
	}
	collection, err := Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := Read(bytes.NewReader(collection))
	if err != nil || len(imported.Requests) != 1 || imported.Requests[0].GraphQL == nil ||
		*imported.Requests[0].GraphQL != *request.GraphQL {
		t.Fatalf("portable collection lost GraphQL semantics: %v", err)
	}
}
