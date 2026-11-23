package execution

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPrepareGraphQLPreservesJSONTypesAndUsesScopedValues(t *testing.T) {
	document := GraphQLDocument{
		Query:         "mutation Update($name: String!) { update(name: $name) { {{field}} } }",
		OperationName: "{{operation}}",
		Variables:     `{"name":"{{name}}","options":{"ids":[9007199254740993,true,null]}}`,
	}
	target, payload, err := document.Prepare("{{base}}/graphql", [][2]string{{"x-trace", "{{trace}}"}, {"accept", "application/json"}},
		map[string]string{"base": "https://example.test", "field": "id", "operation": "Update", "name": "ignored", "trace": "fixture"},
		map[string]string{"name": "测试\"name"})
	if err != nil {
		t.Fatal(err)
	}
	if target != "https://example.test/graphql" || payload.Method != "POST" || len(payload.Headers) != 3 ||
		payload.Headers[0] != [2]string{"x-trace", "fixture"} ||
		payload.Headers[2] != [2]string{"Content-Type", "application/json"} {
		t.Fatalf("unexpected GraphQL HTTP request: %q, %v", target, payload.Headers)
	}
	var wire struct {
		Query         string
		OperationName string
		Variables     map[string]json.RawMessage
	}
	if err := json.Unmarshal([]byte(payload.Body), &wire); err != nil {
		t.Fatal(err)
	}
	if wire.OperationName != "Update" || !strings.Contains(wire.Query, "{ id }") ||
		string(wire.Variables["name"]) != `"测试\"name"` ||
		string(wire.Variables["options"]) != `{"ids":[9007199254740993,true,null]}` {
		t.Fatalf("unexpected GraphQL body: %s", payload.Body)
	}
}

func TestPrepareGraphQLRejectsInvalidVariables(t *testing.T) {
	for _, variables := range []string{`[]`, `null`, `{} {}`, `{"name":"{{missing}}"}`, `{"{{key}}":1,"name":2}`} {
		document := GraphQLDocument{Query: "query { __typename }", Variables: variables}
		if _, _, err := document.Prepare("https://example.test", nil, map[string]string{"key": "name"}); err == nil {
			t.Fatalf("accepted invalid variables %q", variables)
		}
	}
}
