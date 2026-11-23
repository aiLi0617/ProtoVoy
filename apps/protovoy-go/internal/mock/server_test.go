//go:build !js

package mock

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMockReloadPreservesInFlightSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.json")
	old := `[{"id":"fixture","method":"GET","path":"/hello","status":200,"body":"old"}]`
	if err := os.WriteFile(path, []byte(old), 0600); err != nil {
		t.Fatal(err)
	}
	app := &service{path: path, hits: make(map[string]uint64)}
	route := "/hello"
	request := Request{ProjectKey: "default-project", ServiceKey: "default", Method: "GET", Path: &route}
	selected, _, _ := app.selectRule(request)
	if selected == nil {
		t.Fatal("initial rule not selected")
	}
	next := `[{"id":"fixture","method":"GET","path":"/hello","status":200,"body":"replacement"}]`
	if err := os.WriteFile(path, []byte(next), 0600); err != nil {
		t.Fatal(err)
	}
	current, _, hits := app.selectRule(request)
	if current == nil || current.Body != "replacement" || selected.Body != "old" || hits != 2 {
		t.Fatalf("reload did not preserve snapshot and hit count: old=%v new=%v hits=%d", selected, current, hits)
	}
}
func TestMockHTTPRoutesAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.json")
	write := func(contents string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(`[{"id":"fixture","projectKey":"project","serviceKey":"api","operationId":"lookup","method":"GET","path":"/users/{id}","status":201,"body":"first","errorEvery":2}]`)
	app, err := NewHandler(path, "127.0.0.1:39218", "test")
	if err != nil {
		t.Fatal(err)
	}
	check := func(route string, status int, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		app.ServeHTTP(w, httptest.NewRequest(http.MethodGet, route, nil))
		if w.Code != status || w.Body.String() != body {
			t.Fatalf("%s returned %d %q, want %d %q", route, w.Code, w.Body, status, body)
		}
		return w
	}
	if got := check("/m1/project/api/users/42", 201, "first").Header().Get("X-ProtoVoy-Mock-Scenario"); got != "fixture" {
		t.Fatalf("scenario header = %q", got)
	}
	write(`[`)
	if got := check("/m2/project/api/lookup", 503, "Injected mock failure\n").Header().Get("X-ProtoVoy-Mock-Injected"); got != "true" {
		t.Fatalf("injection header = %q", got)
	}
	health := httptest.NewRecorder()
	app.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if !strings.Contains(health.Body.String(), `"rulesReloadFailed":true`) {
		t.Fatalf("health did not report rejected reload: %s", health.Body)
	}
	write(`[{"id":"fixture","projectKey":"project","serviceKey":"api","operationId":"lookup","method":"GET","path":"/users/{id}","status":202,"body":"second"}]`)
	check("/m2/project/api/lookup", 202, "second")
	health = httptest.NewRecorder()
	app.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if !strings.Contains(health.Body.String(), `"rulesReloadFailed":false`) {
		t.Fatalf("health did not report reload recovery: %s", health.Body)
	}
	check("/m2/project/api/other", 404, "No ProtoVoy Mock response matched\n")
}

func TestMockRejectsInvalidResponseHeaders(t *testing.T) {
	for _, headers := range []string{
		`{"bad name":"value"}`,
		`{"X-Test":"value\r\nInjected: yes"}`,
		`{"X-Test":"value\u0000"}`,
	} {
		path := filepath.Join(t.TempDir(), "rules.json")
		data := `[{"id":"fixture","method":"GET","path":"/","status":200,"headers":` + headers + `}]`
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := NewHandler(path, "127.0.0.1:39218", "test"); err == nil {
			t.Fatalf("invalid headers accepted: %s", headers)
		}
	}
}

func TestMockRejectsUnknownRuleFields(t *testing.T) {
	for _, extra := range []string{
		`"matchConditons":{"query":{"mode":"test"}}`,
		`"matchConditions":{"bodyContans":"required"}`,
	} {
		path := filepath.Join(t.TempDir(), "rules.json")
		data := `[{"id":"fixture","method":"GET","path":"/","status":200,` + extra + `}]`
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := NewHandler(path, "127.0.0.1:39218", "test"); err == nil {
			t.Fatalf("unknown fields accepted: %s", extra)
		}
	}
}

func TestMockRejectsInvalidConditions(t *testing.T) {
	for _, condition := range []string{
		`{"source":"query","operator":"equal","value":"test"}`,
		`{"source":"query","operator":"matches","value":"["}`,
		`{"source":"query","operator":"equals"}`,
		`{"source":"unknown","operator":"notExists"}`,
		`{"source":"query","operator":"greaterThan","value":"many"}`,
		`{"source":"query","operator":"lessThan","value":"NaN"}`,
		`{"source":"query","operator":"lessOrEqual","value":"+Inf"}`,
	} {
		path := filepath.Join(t.TempDir(), "rules.json")
		data := `[{"id":"fixture","method":"GET","path":"/","status":200,"matchConditions":{"conditions":[` + condition + `]}}]`
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := NewHandler(path, "127.0.0.1:39218", "test"); err == nil {
			t.Fatalf("invalid condition accepted: %s", condition)
		}
	}
}
