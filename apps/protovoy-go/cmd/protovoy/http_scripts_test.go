package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

func TestHTTPCLIRequestAndResponseScripts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jobs" || r.Header.Get("X-Script") != "ready" {
			t.Error("pre-request changes did not reach server")
		}
		w.Header().Set("X-Result", "ok")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	}))
	defer server.Close()
	dir := t.TempDir()
	writeScript := func(name, source string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	pre := writeScript("pre.js", `request.target += '/jobs'; variables.step = 'ready'; request.payload.headers.push(['X-Script', variables.step]);`)
	post := writeScript("post.js", `assert(response.status === 201 && response.body === 'created' && response.headers['x-result'] === 'ok' && variables.step === 'ready');`)
	request := execution.HTTPRequest{Method: http.MethodGet, Target: server.URL}
	var body, status bytes.Buffer
	if err := runScriptedHTTP(t.Context(), request, pre, post, &body, &status); err != nil ||
		body.String() != "created" || status.String() != "HTTP 201\n" {
		t.Fatalf("scripted request returned %q / %q, %v", body.String(), status.String(), err)
	}
	body.Reset()
	status.Reset()
	bad := writeScript("failure.js", `assert(response.status === 200);`)
	if err := runScriptedHTTP(t.Context(), request, pre, bad, &body, &status); err == nil ||
		!strings.Contains(err.Error(), "post-response script") || body.Len() != 0 || status.Len() != 0 {
		t.Fatalf("failed assertion did not prevent successful output: %v", err)
	}
}

func TestCollectionReportsPostScriptFailureWithHTTPStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Step") != "request" {
			t.Error("collection script did not receive request variable precedence")
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	value := project.Project{CollectionVariables: map[string]string{"step": "collection"},
		Requests: []project.Request{{Name: "Jobs", URL: server.URL, Method: "GET", Variables: map[string]string{"step": "request"},
			PreScripts:  []string{`request.payload.headers.push(['X-Step', variables.step]);`},
			PostScripts: []string{`assert(response.status === 200);`},
		}}}
	data, err := project.Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	value, err = project.Read(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err = runCollection(t.Context(), value, collectionRunOptions{}, &output)
	if err == nil || !strings.Contains(output.String(), `"status":201,"passed":false`) ||
		!strings.Contains(output.String(), "post-response script") {
		t.Fatalf("collection did not report the failed assertion: %v, %s", err, &output)
	}
}
