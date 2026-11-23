package project

import (
	"strings"
	"testing"

	"protovoy.dev/protovoy/internal/execution"
)

func TestReadPortableProjectAndPrepareHTTP(t *testing.T) {
	value, err := Read(strings.NewReader(`{
		"format":"protovoy-project","version":1,"name":"Demo",
		"exportedAt":"2025-01-01T00:00:00.000Z",
		"requests":[{
			"name":"Create","method":"POST","url":"{{ base }}/users",
			"headers":{"X-Name":"{{name}}","Content-Type":"application/json"},
			"body":"{\"name\":\"{{name}}\"}","collectionPath":["Users","Write"],
			"variables":{"base":"http://localhost:8080","name":"测试"}
		}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	request := value.Requests[0]
	if value.Name != "Demo" || request.Name != "Create" || strings.Join(request.CollectionPath, "/") != "Users/Write" {
		t.Fatalf("unexpected collection: %+v", value)
	}
	headers, err := execution.ParseHeaders(request.HeaderText())
	if err != nil {
		t.Fatal(err)
	}
	target, payload, err := (execution.HTTPPayload{Method: request.Method, Headers: headers, Body: request.Body}).
		ResolveVariables(request.URL, request.Variables)
	if err != nil || target != "http://localhost:8080/users" || payload.Method != "POST" ||
		payload.Body != `{"name":"测试"}` || len(payload.Headers) != 2 || payload.Headers[1] != [2]string{"X-Name", "测试"} {
		t.Fatalf("project request failed to resolve: %v, %q, %+v", err, target, payload)
	}
}

func TestReadRejectsInvalidProject(t *testing.T) {
	for _, input := range []string{
		`{"format":"protovoy-project","version":2,"requests":[]}`,
		`{"format":"protovoy-project","requests":[null]}`,
		`{"format":"protovoy-project","requests":[{"url":"/","headers":{"X-Name":"a\nX-Injected: b"}}]}`,
		`{"format":"protovoy-project","requests":[]}` + strings.Repeat(" ", MaxBytes),
	} {
		if _, err := Read(strings.NewReader(input)); err == nil {
			t.Fatal("invalid project was accepted")
		}
	}
}
