package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

func TestCollectionResponseAssertionUsesTransformedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "before")
	}))
	defer server.Close()
	request := project.Request{URL: server.URL, Method: http.MethodGet, ExpectedHTTPStatus: 201, ResponseBodyContains: "after"}
	options := collectionRunOptions{transformResponse: func(_ context.Context, input string) (string, error) {
		if input != "before" {
			t.Fatalf("unexpected transformer input %q", input)
		}
		return "after", nil
	}}
	if err := runCollection(t.Context(), project.Project{Requests: []project.Request{request}}, options, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestCollectionReportsUnexpectedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	request := project.Request{Name: "missing", URL: server.URL, Method: http.MethodGet}
	expected := request
	expected.ExpectedHTTPStatus = http.StatusNotFound
	var output bytes.Buffer
	err := runCollection(t.Context(), project.Project{Requests: []project.Request{request, expected}}, collectionRunOptions{}, &output)
	if err == nil || err.Error() != "1 collection request(s) failed" {
		t.Fatalf("unexpected run error: %v", err)
	}
	decoder := json.NewDecoder(&output)
	var failed, passed collectionResult
	if err := decoder.Decode(&failed); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&passed); err != nil {
		t.Fatal(err)
	}
	if failed.Passed || failed.RequestIndex != 1 || failed.Status != http.StatusNotFound || failed.Error != "unexpected response status 404" {
		t.Fatalf("unexpected failed result: %+v", failed)
	}
	if !passed.Passed || passed.RequestIndex != 2 || passed.Error != "" {
		t.Fatalf("unexpected passed result: %+v", passed)
	}
}

func TestCollectionResolvesBodyAssertionPerRow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.URL.Query().Get("value"))
	}))
	defer server.Close()
	value := project.Project{Requests: []project.Request{{
		Name: "echo", URL: server.URL + "/?value={{value}}", Method: http.MethodGet,
		ResponseBodyContains: "{{expected}}",
	}}}
	var output bytes.Buffer
	err := runCollection(t.Context(), value, collectionRunOptions{rows: []map[string]string{
		{"value": "first", "expected": "first"},
		{"value": "second", "expected": "second"},
	}}, &output)
	if err != nil || strings.Count(output.String(), `"passed":true`) != 2 {
		t.Fatalf("unexpected collection results: %v\n%s", err, output.String())
	}
}

func TestCollectionSearchesLargeHTTPBody(t *testing.T) {
	const needle = "needle"
	body := strings.Repeat("x", (8<<20)+(32<<10)-1) + needle
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer server.Close()
	status, err := collectHTTPStatus(t.Context(), execution.HTTPClient{},
		execution.HTTPRequest{Target: server.URL, Method: http.MethodGet}, "http", needle)
	if err != nil || status != http.StatusOK {
		t.Fatalf("large body assertion returned HTTP %d: %v", status, err)
	}
	// Start the match at the last byte of the initial read.
	splitBody := strings.Repeat("x", (32<<10)+len(needle)-2) + needle
	found, err := streamContains(strings.NewReader(splitBody), []byte(needle))
	if err != nil || !found {
		t.Fatalf("match across read boundary: found=%v, error=%v", found, err)
	}
}

func TestCollectionDoesNotSendWithInvalidBodyAssertion(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	value := project.Project{Requests: []project.Request{{
		Name: "write", URL: server.URL, Method: http.MethodPost,
		ResponseBodyContains: "{{expected}}",
	}}}
	var output bytes.Buffer
	err := runCollection(t.Context(), value, collectionRunOptions{rows: []map[string]string{
		{},
		{"expected": ""},
		{"expected": strings.Repeat("x", project.MaxResponseBodyAssertionBytes+1)},
	}}, &output)
	if err == nil || calls.Load() != 0 ||
		!strings.Contains(output.String(), "could not resolve response body assertion") ||
		strings.Count(output.String(), "resolved response body assertion must contain") != 2 {
		t.Fatalf("calls=%d, error=%v, results=%s", calls.Load(), err, output.String())
	}
}

func TestCollectionRejectsTruncatedBodyAfterMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "4096")
		_, _ = io.WriteString(w, "needle")
		w.(http.Flusher).Flush()
	}))
	defer server.Close()
	status, err := collectHTTPStatus(t.Context(), execution.HTTPClient{},
		execution.HTTPRequest{Target: server.URL, Method: http.MethodGet}, "http", "needle")
	if status != http.StatusOK || err == nil || err.Error() != "could not read the complete HTTP response" {
		t.Fatalf("truncated response returned HTTP %d: %v", status, err)
	}
}
