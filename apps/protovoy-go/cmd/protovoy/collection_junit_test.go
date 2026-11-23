package main

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"protovoy.dev/protovoy/internal/project"
)

func TestFailFastJUnitReportsSkippedRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	request := project.Request{Name: "fixture", URL: server.URL, Method: http.MethodGet}
	value := project.Project{Requests: []project.Request{request, request}}
	path := filepath.Join(t.TempDir(), "report.xml")
	options := collectionRunOptions{junitPath: path, failFast: true, rows: []map[string]string{{}, {}}}
	if err := runCollection(t.Context(), value, options, io.Discard); err == nil {
		t.Fatal("expected the first request to fail")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Cases []struct {
			Failure string `xml:"failure"`
			Skipped *struct {
				Message string `xml:"message,attr"`
			} `xml:"skipped"`
		} `xml:"testcase"`
	}
	if err := xml.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Cases) != 2 || report.Cases[0].Failure == "" || report.Cases[1].Skipped == nil ||
		report.Cases[1].Skipped.Message != "3 request(s) skipped after the first failure" {
		t.Fatalf("unexpected JUnit report: %s", data)
	}
}

func TestCancelledCollectionReportsJUnitError(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	path := filepath.Join(t.TempDir(), "report.xml")
	value := project.Project{Requests: []project.Request{{Name: "fixture", URL: "http://example.test"}}}
	if err := runCollection(ctx, value, collectionRunOptions{junitPath: path}, io.Discard); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		XMLName xml.Name `xml:"testsuite"`
		Error   string   `xml:"testcase>error"`
	}
	if err := xml.Unmarshal(data, &report); err != nil || report.Error == "" {
		t.Fatalf("expected a JUnit execution error, got %s (%v)", data, err)
	}
}
