package mock

import (
	"net/http"
	"net/url"
	"slices"
	"testing"
)

func TestSelectScenario(t *testing.T) {
	path, idPattern, minimum := "/users/42", "^[0-9]+$", "2"
	request := Request{
		ProjectKey: "project", ServiceKey: "api", Method: "POST", Path: &path,
		Query: url.Values{"mode": {"test"}}, Headers: http.Header{"X-Env": {"local"}},
		Body: `{"items":[{"count":3}]}`,
	}
	first := Rule{
		ID: "a", ProjectKey: "project", ServiceKey: "api", OperationID: "update",
		Method: "post", Path: "/users/{id}", Priority: 10,
		Match: Conditions{
			Query: map[string]string{"mode": "test"}, Headers: map[string]string{"x-env": "local"},
			Conditions: []Condition{
				{Source: "path", Name: "id", Operator: "matches", Value: &idPattern},
				{Source: "body", Name: "$.items[0].count", Operator: "greaterThan", Value: &minimum},
			},
		},
	}
	second := first
	second.ID, second.OperationID, second.Priority = "b", "replace", 1
	rules := []Rule{second, first}
	if winner, conflict := Select(rules, request); winner != nil || !slices.Equal(conflict, []string{"a", "b"}) {
		t.Fatalf("expected conflict across operations, got winner=%v conflict=%v", winner, conflict)
	}
	request.OperationID = "update"
	if winner, conflict := Select(rules, request); winner == nil || winner.ID != "a" || len(conflict) != 0 {
		t.Fatalf("expected explicit operation to select scenario a, got winner=%v conflict=%v", winner, conflict)
	}
	rules[0].OperationID = "update"
	if winner, _ := Select(rules, request); winner == nil || winner.ID != "a" {
		t.Fatalf("expected higher priority scenario a, got %v", winner)
	}
	request.Body = `{"items":[{"count":1}]}`
	if winner, conflict := Select(rules, request); winner != nil || len(conflict) != 0 {
		t.Fatalf("expected failed JSON condition to exclude both rules, got winner=%v conflict=%v", winner, conflict)
	}
}
