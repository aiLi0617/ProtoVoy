package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/iotest"

	"github.com/kjanat/wacogo/wasmtools"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

func TestPluginInputDistinguishesReadFailureFromInvalidText(t *testing.T) {
	for _, test := range []struct {
		input   io.Reader
		message string
	}{
		{iotest.ErrReader(io.ErrUnexpectedEOF), "could not read plugin input"},
		{strings.NewReader("\xff"), "plugin input must be UTF-8 within 1 MiB"},
	} {
		value, err := readPluginInput(test.input)
		if err == nil || err.Error() != test.message || value != "" {
			t.Fatalf("unexpected plugin input result: %q, %v", value, err)
		}
	}
	if _, err := readPluginInput(iotest.ErrReader(os.ErrDeadlineExceeded)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected a timeout, got %v", err)
	}
}

func TestSignedResponsePluginUsesPreparedSnapshot(t *testing.T) {
	ctx := t.Context()
	tool, err := wasmtools.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tool.Close(ctx)
	wat, err := os.ReadFile("../../internal/plugins/testdata/echo.wat")
	if err != nil {
		t.Fatal(err)
	}
	component, err := tool.Parse(ctx, wat)
	if err != nil {
		t.Fatal(err)
	}
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	const metadata = `{"description":"","id":"echo","kind":"transformer","name":"Echo","permissions":[],"publisherKeyId":"fixture","version":"1"}`
	digest := sha256.Sum256(component)
	payload := append([]byte("ProtoVoy Plugin Signature v1\x00"), digest[:]...)
	signature := ed25519.Sign(private, append(payload, metadata...))
	manifest := metadata[:len(metadata)-1] + `,"signatureBase64":"` + base64.StdEncoding.EncodeToString(signature) + `"}`
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin.wasm")
	if err := os.WriteFile(path, component, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
	keys, err := json.Marshal(map[string][]byte{"fixture": public})
	if err != nil {
		t.Fatal(err)
	}
	transform, err := prepareResponsePlugin(dir, string(keys))
	if err != nil {
		t.Fatal(err)
	}
	var removeOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		removeOnce.Do(func() {
			if err := os.Remove(path); err != nil {
				t.Error(err)
			}
		})
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "响应正文")
	}))
	defer server.Close()
	var output, status bytes.Buffer
	err = runResponsePlugin(ctx, execution.HTTPRequest{Target: server.URL}, dir, string(keys), &output, &status)
	if err != nil || output.String() != "响应正文" || status.String() != "HTTP 201\n" {
		t.Fatalf("signed response plugin returned %q, %q, %v", &output, &status, err)
	}
	output.Reset()
	collection := project.Project{Requests: []project.Request{{
		Name: "Signed transformer", URL: server.URL, Method: http.MethodGet,
		ExpectedHTTPStatus: http.StatusCreated, ResponseBodyContains: "响应正文",
	}}}
	options := collectionRunOptions{
		transformResponse: transform, concurrency: 2, rows: []map[string]string{{}, {}},
	}
	if err := runCollection(ctx, collection, options, &output); err != nil {
		t.Fatal(err)
	}
	results := json.NewDecoder(&output)
	for iteration := 1; iteration <= 2; iteration++ {
		var item collectionResult
		if err := results.Decode(&item); err != nil || !item.Passed || item.Iteration != iteration {
			t.Fatalf("unexpected signed transformer result: %+v, %v", item, err)
		}
	}
}

func TestResponsePluginRejectsInlineCredentialsBeforeSending(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("sent an HTTP request with credentials")
	}))
	defer server.Close()
	for _, request := range []execution.HTTPRequest{
		{Target: server.URL, Headers: [][2]string{{"aUtHoRiZaTiOn", "fixture"}}},
		{Target: strings.Replace(server.URL, "://", "://fixture:password@", 1)},
		{Target: server.URL + "/?%61pi_key=fixture"},
	} {
		code, err := transformHTTPResponse(t.Context(), execution.HTTPClient{}, request, nil, nil, nil)
		if code != 0 || err == nil || !strings.Contains(err.Error(), "credential") {
			t.Fatalf("unexpected result for authenticated request: %d, %v", code, err)
		}
	}
}

func TestResponsePluginDoesNotReceiveCredentialBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"access_token":"fixture"}`)
	}))
	defer server.Close()
	code, err := transformHTTPResponse(t.Context(), execution.HTTPClient{}, execution.HTTPRequest{Target: server.URL},
		func(context.Context, string) (string, error) {
			t.Fatal("plugin received a possible credential")
			return "", nil
		}, nil, nil)
	if code != http.StatusOK || err == nil || !strings.Contains(err.Error(), "possible credentials") {
		t.Fatalf("expected response credential rejection, got %d, %v", code, err)
	}
}

func TestResponsePluginRejectsCredentialRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			t.Error("followed redirect with credentials")
			return
		}
		http.Redirect(w, r, "/next?api_key=fixture", http.StatusFound)
	}))
	defer server.Close()
	request := execution.HTTPRequest{Target: server.URL, FollowRedirects: true}
	_, err := transformHTTPResponse(t.Context(), execution.HTTPClient{}, request, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "URL credentials") {
		t.Fatalf("expected credential rejection, got %v", err)
	}
}

func TestResponsePluginRejectsUnsignedPackageBeforeSending(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("sent HTTP request before validating plugin trust")
	}))
	defer server.Close()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"id":"fixture","name":"Fixture","version":"1","kind":"transformer"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.wasm"), []byte("\x00asm\x0d\x00\x01\x00"), 0600); err != nil {
		t.Fatal(err)
	}
	var output, status bytes.Buffer
	err := runResponsePlugin(t.Context(), execution.HTTPRequest{Target: server.URL}, dir, "", &output, &status)
	if err == nil || !strings.Contains(err.Error(), "trusted publisher signature") || output.Len() != 0 || status.Len() != 0 {
		t.Fatalf("unexpected result for unsigned transformer: %v", err)
	}
}

func TestImporterPluginEmitsOnlyValidatedPortableProject(t *testing.T) {
	ctx := t.Context()
	tool, err := wasmtools.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tool.Close(ctx)
	wat, err := os.ReadFile("../../internal/plugins/testdata/echo.wat")
	if err != nil {
		t.Fatal(err)
	}
	source := strings.NewReplacer(`export "transform"`, `export "import"`, `param "input"`, `param "document"`).Replace(string(wat))
	component, err := tool.Parse(ctx, []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	files := map[string][]byte{
		"manifest.json": []byte(`{"id":"fixture","name":"Importer","version":"1","kind":"importer"}`),
		"plugin.wasm":   component,
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(directory, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	args := []string{"-allow-unsigned", directory}
	var output bytes.Buffer
	const valid = `{"format":"protovoy-project","version":1,"requests":[{"name":"测试","url":"https://example.test","method":"GET","headers":{}}]}`
	if err := importPluginProject(ctx, args, "", strings.NewReader(valid), &output); err != nil {
		t.Fatal(err)
	}
	collection, err := project.Read(&output)
	if err != nil || len(collection.Requests) != 1 || collection.Requests[0].Name != "测试" {
		t.Fatalf("unexpected imported project: %+v, %v", collection, err)
	}
	for _, invalid := range []string{
		"invalid output",
		`{"format":"protovoy-multiprotocol-project","version":2,"requests":[{"url":"https://example.test","method":"GET","preScripts":["request.method = 'POST'"]}]}`,
		`{"protocolId":"http","target":"https://example.test","payload":{"type":"http","method":"GET"}}`,
		strings.Replace(valid, `"headers":{}`, `"headers":{"Authorization":"fixture-value"}`, 1),
	} {
		output.Reset()
		if err := importPluginProject(ctx, args, "", strings.NewReader(invalid), &output); err == nil || output.Len() != 0 {
			t.Fatal("importer wrote invalid or sensitive project data")
		}
	}
}
