package main

import (
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"strings"
	"testing"
	"time"
)

func TestAgentPreparesLegacyMultipartWithVariables(t *testing.T) {
	envelope := requestEnvelope{
		Protocol: "http", Target: "http://localhost/upload",
		Variables: map[string]string{"name": "attachment", "file": "export.bin", "data": "AP8="},
	}
	var payload httpEnvelopePayload
	data := json.RawMessage(`{"type":"http","method":"POST","multipart":[{"name":"{{name}}","fileName":"{{file}}","value":"{{data}}","base64":true},{"name":"label","value":"second"}]}`)
	if !decodeExecutionPayload(data, &payload) {
		t.Fatal("could not parse the legacy multipart payload")
	}
	request, err := envelope.prepareHTTP(payload, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	media, params, err := mime.ParseMediaType(request.Headers[0][1])
	if err != nil || media != "multipart/form-data" || params["boundary"] == "" {
		t.Fatalf("invalid multipart content type: %v", request.Headers)
	}
	reader := multipart.NewReader(strings.NewReader(request.Body), params["boundary"])
	for _, expected := range []struct{ name, filename, body string }{
		{"attachment", "export.bin", "\x00\xff"}, {"label", "", "second"},
	} {
		part, err := reader.NextPart()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(part)
		if err != nil || part.FormName() != expected.name || part.FileName() != expected.filename || string(body) != expected.body {
			t.Fatal("multipart part differs from the resolved request")
		}
	}
	if _, err := reader.NextPart(); err != io.EOF {
		t.Fatal("unexpected trailing multipart data")
	}
}
