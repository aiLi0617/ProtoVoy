package project

import (
	"fmt"
	"strings"
	"testing"
)

func TestPostmanImportRejectsActiveScriptsAtEveryScope(t *testing.T) {
	const event = `"event":[{"listen":"prerequest","script":{"exec":["pm.environment.set('host', 'example.test')"]}}],`
	const collection = `{"info":{"schema":"https://schema.getpostman.com/json/collection/v2.1.0/collection.json"},%s"item":[{%s"name":"Folder","item":[{%s"request":"https://example.test"}]}]}`
	for _, source := range []string{
		fmt.Sprintf(collection, event, "", ""),
		fmt.Sprintf(collection, "", event, ""),
		fmt.Sprintf(collection, "", "", event),
	} {
		if _, err := Read(strings.NewReader(source)); err == nil || !strings.Contains(err.Error(), "contains scripts") {
			t.Fatalf("expected an explicit unsupported script error, got %v", err)
		}
	}
	disabled := strings.Replace(event, `"listen":`, `"disabled":true,"listen":`, 1)
	if _, err := Read(strings.NewReader(fmt.Sprintf(collection, disabled, "", ""))); err != nil {
		t.Fatalf("disabled script prevented import: %v", err)
	}
}

func TestReadPostman(t *testing.T) {
	const source = `{"info":{"name":"Fixture","schema":"https://schema.getpostman.com/json/collection/v2.1.0/collection.json"},
		"variable":[{"key":"base","value":"https://example.test"}],
		"item":[{"name":"Folder","item":[{"name":"Create","request":{
			"method":"POST","url":{"raw":"{{base}}/create"},
			"header":[{"key":"Authorization","value":"fixture"},{"key":"X-Disabled","value":"fixture","disabled":true}],
			"body":{"mode":"raw","raw":"{\"name\":\"fixture\"}","options":{"raw":{"language":"json"}}}
		}},{"name":"Empty","item":[]}]}]}`
	value, err := Read(strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	if len(value.Requests) != 1 || len(value.EmptyFolders) != 1 ||
		strings.Join(value.EmptyFolders[0], "/") != "Folder/Empty" {
		t.Fatalf("unexpected collection: %+v", value)
	}
	request := value.Requests[0]
	if request.URL != "{{base}}/create" || strings.Join(request.CollectionPath, "/") != "Folder" ||
		len(request.Headers) != 1 || request.Headers["Content-Type"] != "application/json" {
		t.Fatalf("unexpected request: %+v", request)
	}
	input, err := request.PrepareCollectionHTTP(value.CollectionVariables)
	if err != nil || input.Target != "https://example.test/create" || input.Body != `{"name":"fixture"}` {
		t.Fatalf("unexpected execution input: %+v, %v", input, err)
	}
}
