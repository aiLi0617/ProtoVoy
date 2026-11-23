package project

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestReadOpenAPIOperations(t *testing.T) {
	const document = `{
	  "openapi":"3.1.0","info":{"title":"Example API"},
	  "servers":[{"url":"https://{region}.example.test/api","variables":{"region":{"default":"eu"}}}],
	  "paths":{"/items/{id}":{
	    "parameters":[{"name":"id","in":"path","example":"one/two"},{"name":"limit","in":"query","example":10}],
	    "post":{"summary":"Create item","tags":["Items"],
	      "parameters":[{"name":"limit","in":"query","example":20}],
	      "requestBody":{"content":{"application/json":{"example":{"name":"fixture"}}}}}
	  }}
	}`
	project, err := Read(strings.NewReader(document))
	if err != nil {
		t.Fatal(err)
	}
	if project.Name != "Example API" || len(project.Requests) != 1 {
		t.Fatalf("unexpected project: %+v", project)
	}
	request := project.Requests[0]
	input, err := request.PrepareCollectionHTTP(nil)
	if err != nil {
		t.Fatal(err)
	}
	if request.Name != "Create item" || strings.Join(request.CollectionPath, "/") != "Items" ||
		input.Method != "POST" || input.Target != "https://eu.example.test/api/items/one%2Ftwo?limit=20" ||
		!strings.Contains(input.Body, `"name": "fixture"`) || request.Headers["Content-Type"] != "application/json" {
		t.Fatalf("unexpected request: %+v", input)
	}
}

func TestReadOpenAPIRelativeServer(t *testing.T) {
	project, err := Read(strings.NewReader(`{"openapi":"3.0.3","servers":[{"url":"/api"}],"paths":{"/items":{"get":{}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	input, err := project.Requests[0].PrepareCollectionHTTP(map[string]string{"baseUrl": "https://example.test"})
	if err != nil || input.Target != "https://example.test/api/items" {
		t.Fatalf("unexpected relative-server request: %+v, %v", input, err)
	}
}

func TestReadOpenAPILocalReferences(t *testing.T) {
	const document = `{"openapi":"3.1.0","servers":[{"url":"https://example.test"}],
		"paths":{"/items":{"$ref":"#/components/pathItems/Items"}},
		"components":{
			"pathItems":{"Items":{"post":{
				"parameters":[{"$ref":"#/components/parameters/query~1size%20~0"}],
				"requestBody":{"$ref":"#/components/requestBodies/Body"}
			}}},
			"parameters":{"query/size ~":{"name":"size","in":"query","schema":{"$ref":"#/components/schemas/Size"}}},
			"schemas":{"Size":{"type":"integer","default":3}},
			"examples":{"Fixture":{"value":{"name":"fixture"}}},
			"requestBodies":{"Body":{"content":{"application/json":{"examples":{"fixture":{"$ref":"#/components/examples/Fixture"}}}}}}
		}}`
	value, err := Read(strings.NewReader(document))
	if err != nil {
		t.Fatal(err)
	}
	request := value.Requests[0]
	if request.URL != "https://example.test/items?size=3" || request.Headers["Content-Type"] != "application/json" ||
		!strings.Contains(request.Body, `"name": "fixture"`) {
		t.Fatalf("unexpected request: %+v", request)
	}
}

func TestReadOpenAPIRejectsInvalidReferences(t *testing.T) {
	for _, reference := range []string{"#/paths/~1items", "#/paths/missing", "https://example.test/spec.json#/paths/item"} {
		document := `{"openapi":"3.1.0","paths":{"/items":{"$ref":"` + reference + `"}}}`
		if _, err := Read(strings.NewReader(document)); err == nil {
			t.Errorf("accepted invalid reference %q", reference)
		}
	}
}

func TestReadOpenAPIGeneratesJSONBody(t *testing.T) {
	const document = `{"openapi":"3.1.0","servers":[{"url":"https://example.test"}],
		"paths":{"/items":{"post":{"requestBody":{"content":{"application/json":{
			"schema":{"$ref":"#/components/schemas/Item"}
		}}}}}},
		"components":{"schemas":{"Item":{"type":"object","properties":{
			"id":{"type":"integer","readOnly":true},
			"enabled":{"type":"boolean"},
			"kind":{"type":"string","enum":["fixture"]},
			"names":{"type":"array","items":{"type":"string"}},
			"count":{"type":["null","integer"]},
			"password":{"type":"string"}
		}}}}}`
	value, err := Read(strings.NewReader(document))
	if err != nil {
		t.Fatal(err)
	}
	request := value.Requests[0]
	var body bytes.Buffer
	if err := json.Compact(&body, []byte(request.Body)); err != nil {
		t.Fatal(err)
	}
	if body.String() != `{"count":0,"enabled":false,"kind":"fixture","names":["string"]}` {
		t.Fatalf("unexpected generated body: %s", body.String())
	}
}

func TestOpenAPIComposedBody(t *testing.T) {
	document := []byte(`{"components":{"schemas":{"Base":{"properties":{"name":{"type":"string"}}}}}}`)
	references := openAPIReferences{document: document, remaining: 2 * MaxBytes}
	body, err := references.generateBody(json.RawMessage(`{
		"allOf":[{"$ref":"#/components/schemas/Base"},{"properties":{"count":{"type":"integer"}}}],
		"oneOf":[{"properties":{"enabled":{"type":"boolean"}}},{"properties":{"unused":{"type":"string"}}}],
		"properties":{"kind":{"type":"string","anyOf":[{"const":"fixture"},{"const":"other"}]}}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"count":0,"enabled":false,"kind":"fixture","name":"string"}` {
		t.Fatalf("unexpected generated body: %s", body)
	}
}

func TestReadOpenAPIYAML(t *testing.T) {
	const source = `openapi: 3.1.0
info:
  title: YAML API
servers:
  - url: https://example.test
paths:
  /items:
    post:
      requestBody:
        $ref: '#/components/requestBodies/Item'
      responses:
        200:
          description: OK
components:
  requestBodies:
    Item:
      content:
        application/json:
          example:
            enabled: true
            count: 2
`
	value, err := Read(strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	if value.Name != "YAML API" || len(value.Requests) != 1 ||
		!strings.Contains(value.Requests[0].Body, `"enabled": true`) {
		t.Fatalf("unexpected YAML project: %+v", value)
	}
}

func TestReadOpenAPIYAMLRejectsAmbiguity(t *testing.T) {
	for _, source := range []string{
		"openapi: 3.1.0\npaths: {}\npaths: {}",
		"openapi: 3.1.0\npaths: &paths\n  /items:\n    get: *paths",
		"openapi: 3.1.0\npaths: {}\n---\nopenapi: 3.1.0",
	} {
		if _, err := Read(strings.NewReader(source)); err == nil {
			t.Errorf("accepted unsupported YAML: %s", source)
		}
	}
}
