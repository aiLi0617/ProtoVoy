package execution

import (
	"encoding/json"
	"testing"
)

func TestHTTPDefaultsPreserveExplicitValues(t *testing.T) {
	payload := HTTPPayload{
		Headers: [][2]string{{"x-region", "local"}, {"Content-Type", "application/json"}},
		Body:    `{"region":"local","number":9007199254740993}`,
	}
	target, result, err := payload.ResolveHTTPDefaults("https://example.test/?region=local", []HTTPDefault{
		{Location: "header", Name: "X-Region", Value: "default"},
		{Location: "header", Name: "X-Locale", Value: "{{locale}}"},
		{Location: "query", Name: "region", Value: "default"},
		{Location: "query", Name: "locale", Value: "{{locale}}"},
		{Location: "body", Name: "region", Value: "default"},
		{Location: "body", Name: "locale", Value: "{{locale}}"},
	}, map[string]string{"locale": "en GB"})
	if err != nil || target != "https://example.test/?region=local&locale=en+GB" {
		t.Fatalf("query defaults: %s, %v", target, err)
	}
	if len(result.Headers) != 3 || result.Headers[0][1] != "local" || result.Headers[2] != [2]string{"X-Locale", "en GB"} {
		t.Fatalf("header defaults: %v", result.Headers)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal([]byte(result.Body), &body); err != nil || string(body["region"]) != `"local"` ||
		string(body["locale"]) != `"en GB"` || string(body["number"]) != "9007199254740993" {
		t.Fatalf("body defaults: %s", result.Body)
	}
}
