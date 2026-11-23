package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/vivaui/viva/text"
	"protovoy.dev/protovoy/internal/project"
)

func TestLegacyHTTPTimeoutRoundTrip(t *testing.T) {
	value, err := project.Read(strings.NewReader(`{"protocolId":"http","target":"http://localhost/","timeoutMs":1250,"payload":{"type":"http","method":"GET"}}`))
	if err != nil {
		t.Fatal(err)
	}
	w := newWorkbench()
	w.collection.project = &value
	w.loadRequest(1, value.Requests[0])
	_, payload, err := w.prepareRequest()
	if err != nil || payload.TimeoutMS != 1250 {
		t.Fatalf("timeout was lost: %d (%v)", payload.TimeoutMS, err)
	}
	w.httpTimeout.SetText("2500")
	w.saveDraft()
	data, err := project.Encode(*w.collection.project)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := project.Read(bytes.NewReader(data))
	if err != nil || reopened.Requests[0].HTTPTimeoutMS != 2500 {
		t.Fatalf("changed timeout did not round-trip: %v", err)
	}
	w.httpTimeout.SetText("300001")
	if _, _, err := w.prepareRequest(); err == nil {
		t.Fatal("accepted a timeout beyond the supported range")
	}
}

func TestLegacyHTTPBodyFormatRoundTrip(t *testing.T) {
	for _, test := range []struct{ mode, fields string }{
		{"base64", `"bodyEncoding":"base64","body":"AP8="`},
		{"multipart", `"headers":[["Content-Type","multipart/form-data"]],"multipart":[{"name":"file","fileName":"example.bin","base64":true,"value":"{{bytes}}"}]`},
	} {
		t.Run(test.mode, func(t *testing.T) {
			input := `{"protocolId":"http","target":"http://localhost/upload","variables":{"bytes":"AP8="},"payload":{"type":"http","method":"POST",` + test.fields + `}}`
			imported, err := project.Read(strings.NewReader(input))
			if err != nil {
				t.Fatal(err)
			}
			w := newWorkbench()
			w.collection.project = &imported
			w.loadRequest(1, imported.Requests[0])
			if w.httpBodyMode != test.mode {
				t.Fatalf("got mode %q", w.httpBodyMode)
			}
			_, payload, err := w.prepareRequest()
			if err != nil {
				t.Fatal(err)
			}
			wire, err := payload.EncodeBody()
			if err != nil || !strings.Contains(wire.Body, "\x00\xff") {
				t.Fatalf("binary body was lost: %v", err)
			}
			w.saveDraft()
			data, err := project.Encode(*w.collection.project)
			if err != nil {
				t.Fatal(err)
			}
			restored, err := project.Read(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			other := newWorkbench()
			other.loadRequest(1, restored.Requests[0])
			if other.draft() != w.draft() {
				t.Fatal("HTTP body format did not round-trip")
			}
		})
	}
}

func TestSaveMultipartOmitsInactiveBody(t *testing.T) {
	value := project.Project{Requests: []project.Request{{
		Name: "Upload", URL: "http://localhost/upload", Method: "POST",
		Body: `{"password":"fixture-value"}`,
	}}}
	w := newWorkbench()
	w.collection.project = &value
	w.loadRequest(1, value.Requests[0])
	w.httpBodyMode = "multipart"
	w.multipartBody.input.Editor = text.NewEditor(`[{"name":"field","value":"example"}]`)
	w.saveDraft()
	saved := w.collection.project.Requests[0]
	if saved.Body != "" || len(saved.Multipart) != 1 {
		t.Fatal("saved request should contain only the active multipart body")
	}
	if _, err := project.Encode(*w.collection.project); err != nil {
		t.Fatalf("multipart project could not be exported: %v", err)
	}
}

func TestLegacyHTTPSourceSurvivesSaveAndClearsAfterBodyEdit(t *testing.T) {
	input := `{"protocolId":"http","target":"http://localhost/rpc","payload":{"type":"http","method":"POST","body":"{\"jsonrpc\":\"2.0\",\"method\":\"ping\",\"id\":1}","bodySource":"{\"type\":\"jsonrpc\",\"method\":\"ping\",\"id\":1}"}}`
	imported, err := project.Read(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	w := newWorkbench()
	w.collection.project = &imported
	w.loadRequest(1, imported.Requests[0])
	original := imported.Requests[0]
	if w.currentHTTPBodySource() != original.BodySource || original.BodySource == "" {
		t.Fatal("legacy source was lost")
	}
	w.saveDraft()
	data, err := project.Encode(*w.collection.project)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := project.Read(bytes.NewReader(data))
	if err != nil || reopened.Requests[0].BodySource != original.BodySource {
		t.Fatalf("legacy source did not round-trip: %v", err)
	}
	_, payload, err := w.prepareRequest()
	if err != nil || payload.Body != original.Body {
		t.Fatalf("source changed the wire body: %v", err)
	}
	source := `{"type":"jsonrpc","method":"health","id":9007199254740993}`
	if err := w.applyHTTPSource("jsonrpc", source); err != nil {
		t.Fatal(err)
	}
	w.saveDraft()
	updated := w.collection.project.Requests[0]
	if updated.BodySource != source || updated.Body != `{"jsonrpc":"2.0","method":"health","id":9007199254740993}` {
		t.Fatal("source edits did not update the saved wire body")
	}
	w.requestBody.input.Editor = text.NewEditor("{}")
	w.saveDraft()
	if w.collection.project.Requests[0].BodySource != "" {
		t.Fatal("the updated body retained stale source")
	}
}

func TestNewMessagePackSourceSetsWireBodyAndDefaults(t *testing.T) {
	w := newWorkbench()
	w.target.SetText("http://localhost/upload")
	w.headers.input.Editor = text.NewEditor("X-Test: fixture")
	before := w.draft()
	w.openHTTPSource("messagepack", "")
	if err := w.applyHTTPSource("messagepack", "{"); err == nil || w.draft() != before || !w.showHTTPBodySource {
		t.Fatal("invalid MessagePack source changed the request")
	}
	const source = `{"a":2}`
	if err := w.applyHTTPSource("messagepack", source); err != nil {
		t.Fatal(err)
	}
	_, payload, err := w.prepareRequest()
	if err != nil || payload.Method != "POST" || payload.Body != "gaFhAg==" || payload.BodyEncoding != "base64" ||
		len(payload.Headers) != 2 || payload.Headers[1] != [2]string{"Content-Type", "application/msgpack"} {
		t.Fatalf("unexpected MessagePack payload: %+v (%v)", payload, err)
	}
	w.saveDraft()
	if w.collection.selected != 1 {
		t.Fatalf("MessagePack request was not saved: %s", w.collection.message)
	}
	restored := newWorkbench()
	restored.loadRequest(1, w.collection.project.Requests[0])
	if restored.currentHTTPBodySource() != source || restored.draft() != w.draft() {
		t.Fatal("new MessagePack request did not round-trip")
	}
}
