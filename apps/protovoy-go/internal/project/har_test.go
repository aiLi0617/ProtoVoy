package project

import (
	"strings"
	"testing"
)

func TestReadHAR(t *testing.T) {
	input := "\xef\xbb\xbf" + `{"log":{"version":"1.2","entries":[
		{"request":{"method":"GET","url":"https://example.test/path?view=full","headers":[
			{"name":":method","value":"GET"},{"name":"Authorization","value":"fixture"},
			{"name":"accept","value":"application/json"}]}},
		{"request":{"method":"POST","url":"https://example.test/form","postData":{
			"mimeType":"application/x-www-form-urlencoded","params":[{"name":"q","value":"a b"},{"name":"q","value":"c"}]}}}
	]}}`
	value, err := Read(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(value.Requests) != 2 || len(value.Requests[0].Headers) != 1 ||
		value.Requests[0].Headers["Accept"] != "application/json" ||
		value.Requests[1].Body != "q=a+b&q=c" {
		t.Fatalf("unexpected HAR project: %+v", value)
	}
}

func TestReadHARRejectsCredentialsInQuery(t *testing.T) {
	input := `{"log":{"version":"1.2","entries":[{"request":{"method":"GET","url":"https://example.test/?%61pi_key=fixture"}}]}}`
	if _, err := Read(strings.NewReader(input)); err == nil {
		t.Fatal("accepted a HAR URL with a credential field")
	}
}
