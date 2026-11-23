package project

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"protovoy.dev/protovoy/internal/execution"
)

func TestCurlSnippetSendsLiteralRequest(t *testing.T) {
	body := "@literal 'quote' $(printf expanded)\nnext line"
	header := "value 'with quotes'"
	received := make(chan *http.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "could not read fixture request", http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(strings.NewReader(string(data)))
		received <- r
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	request := Request{
		Method: http.MethodPost,
		URL:    server.URL + "/a/../b?literal=%5Bvalue%5D",
		Headers: map[string]string{
			"X-Fixture": header,
			"X-Empty":   "",
		},
		Body: body,
	}
	snippet, err := request.CurlSnippet(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(snippet, "--data-raw "+quoteShell(body)) {
		t.Fatalf("expected literal request data, got %q", snippet)
	}
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is unavailable")
	}
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("cURL is unavailable")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, shell, "-c", snippet).CombinedOutput(); err != nil {
		t.Fatalf("cURL command failed: %v\n%s", err, output)
	}
	select {
	case actual := <-received:
		data, err := io.ReadAll(actual.Body)
		if err != nil || string(data) != body || actual.Method != request.Method ||
			actual.RequestURI != "/a/../b?literal=%5Bvalue%5D" || actual.Header.Get("X-Fixture") != header ||
			len(actual.Header.Values("X-Empty")) != 1 || actual.Header.Get("Content-Type") != "" {
			t.Fatalf("unexpected request: %s %s, headers %v, body %q, read error %v",
				actual.Method, actual.RequestURI, actual.Header, data, err)
		}
	default:
		t.Fatal("the local server did not receive the generated request")
	}
}

func TestCurlSnippetMultipartRoundTrip(t *testing.T) {
	const value = "line one\r\nline 'two'"
	request := Request{
		URL: "https://example.test/upload", Method: http.MethodPost,
		Multipart: []execution.MultipartPart{{Name: "field", Value: value}},
	}
	snippet, err := request.CurlSnippet(nil)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := ParseCurl(snippet)
	if err != nil {
		t.Fatal(err)
	}
	mediaType, params, err := mime.ParseMediaType(imported.Headers["Content-Type"])
	if err != nil || mediaType != "multipart/form-data" || params["boundary"] == "" {
		t.Fatalf("missing multipart boundary: %v", err)
	}
	reader := multipart.NewReader(strings.NewReader(imported.Body), params["boundary"])
	part, err := reader.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(part)
	if err != nil || part.FormName() != "field" || string(body) != value {
		t.Fatalf("unexpected multipart part: %q, %v", body, err)
	}
}

func TestCurlSnippetRejectsManualFraming(t *testing.T) {
	for _, header := range []string{"content-length", "Transfer-Encoding"} {
		request := Request{URL: "https://example.test", Method: http.MethodPost, Headers: map[string]string{header: "1"}, Body: "fixture"}
		if snippet, err := request.CurlSnippet(nil); err == nil || snippet != "" {
			t.Fatalf("exported a command with manual %s", header)
		}
	}
}

func TestCurlSnippetRejectsResolvedCredentials(t *testing.T) {
	request := Request{URL: "http://example.test/?{{field}}=fixture", Method: http.MethodGet}
	snippet, err := request.CurlSnippet(map[string]string{"field": "api_key"})
	if err == nil || snippet != "" {
		t.Fatal("generated a command containing a resolved credential field")
	}
}

func TestCurlSnippetRejectsScripts(t *testing.T) {
	for _, request := range []Request{
		{URL: "https://example.test", Method: http.MethodGet, PreScripts: []string{"request.method = 'POST'"}},
		{URL: "https://example.test", Method: http.MethodGet, PostScripts: []string{"assert(response.status === 200)"}},
	} {
		snippet, err := request.CurlSnippet(nil)
		if err == nil || !strings.Contains(err.Error(), "request scripts") || snippet != "" {
			t.Fatalf("expected an explicit script error and no command, got %q, %v", snippet, err)
		}
	}
}

func TestParseCurlRoundTrip(t *testing.T) {
	original := Request{
		URL: "https://example.test/path", Method: http.MethodPost,
		Headers: map[string]string{"X-Empty": "", "X-Fixture": "literal 'quote'"},
		Body:    "@literal 'quote' $(printf expanded)\nnext line", HTTPTimeoutMS: 1250,
	}
	snippet, err := original.CurlSnippet(nil)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := ParseCurl(snippet)
	if err != nil {
		t.Fatal(err)
	}
	if actual.URL != original.URL || actual.Method != original.Method || actual.Body != original.Body ||
		actual.HTTPTimeoutMS != original.HTTPTimeoutMS || len(actual.Headers) != 2 ||
		actual.Headers["X-Fixture"] != original.Headers["X-Fixture"] {
		t.Fatalf("unexpected imported request: %+v", actual)
	}
}

func TestParseCurlRejectsUnsupportedInput(t *testing.T) {
	for _, source := range []string{
		"curl https://example.test -d @local.txt",
		"curl https://example.test --json @local.json",
		"curl https://example.test --json '{}' -d 'name=value'",
		"curl https://example.test --data-urlencode field@local.txt",
		"curl https://example.test --data-urlencode '%61pi_key=fixture'",
		"curl -G https://example.test -d '%61pi_key=fixture'",
		"curl https://example.test -H @headers.txt",
		"curl https://example.test -H 'Authorization: Bearer fixture'",
		"curl https://example.test; echo other",
		"curl \"https://example.test/$(printf path)\"",
		"curl https://example.test -k",
	} {
		if _, err := ParseCurl(source); err == nil {
			t.Errorf("accepted unsupported input %q", source)
		}
	}
}

func TestParseCurlJSON(t *testing.T) {
	request, err := ParseCurl(`curl https://example.test --json '{"name":' --json='"fixture"}' -H 'accept: text/plain'`)
	if err != nil {
		t.Fatal(err)
	}
	if request.Method != http.MethodPost || request.Body != `{"name":"fixture"}` ||
		request.Headers["Content-Type"] != "application/json" || request.Headers["Accept"] != "text/plain" {
		t.Fatalf("unexpected JSON request: %+v", request)
	}
}

func TestParseCurlBrowserCopy(t *testing.T) {
	source := "curl --url='https://example.test/path?x=1&y=2' \\\n" +
		"  --header='accept-encoding: gzip, br' -H 'Accept: application/json' \\\n" +
		"  --data-raw='{\"value\":\"a=b\"}' --max-time=1.25 --compressed"
	request, err := ParseCurl(source)
	if err != nil {
		t.Fatal(err)
	}
	if request.URL != "https://example.test/path?x=1&y=2" || request.Method != http.MethodPost ||
		request.Body != `{"value":"a=b"}` || request.HTTPTimeoutMS != 1250 ||
		request.Headers["Accept"] != "application/json" {
		t.Fatalf("unexpected imported request: %+v", request)
	}
	if _, exists := request.Headers["Accept-Encoding"]; exists {
		t.Fatal("compression negotiation should be left to the execution transport")
	}
}

func TestParseCurlFormData(t *testing.T) {
	const fields = ` --data-urlencode 'q=a b+c' --data-urlencode '=x@y' -d 'page=2'`
	request, err := ParseCurl(`curl 'https://example.test/path?existing=1#fragment'` + fields)
	if err != nil {
		t.Fatal(err)
	}
	if request.Method != http.MethodPost || request.Body != "q=a+b%2Bc&x%40y&page=2" ||
		request.Headers["Content-Type"] != "application/x-www-form-urlencoded" {
		t.Fatalf("unexpected form request: %+v", request)
	}
	request, err = ParseCurl(`curl -G 'https://example.test/path?existing=1#fragment'` + fields)
	if err != nil {
		t.Fatal(err)
	}
	if request.Method != http.MethodGet || request.Body != "" || len(request.Headers) != 0 ||
		request.URL != "https://example.test/path?existing=1&q=a+b%2Bc&x%40y&page=2#fragment" {
		t.Fatalf("unexpected query request: %+v", request)
	}
}
