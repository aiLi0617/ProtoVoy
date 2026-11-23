package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"protovoy.dev/protovoy/internal/agentclient"
	"protovoy.dev/protovoy/internal/execution"
)

func TestAgentResponseFailureDiscardsExportBytes(t *testing.T) {
	for _, failure := range []error{context.Canceled, context.DeadlineExceeded, errors.New("incomplete response")} {
		response := agentclient.Response{
			Status: 200, Preview: `{"partial":true}`, PreviewBytes: 16,
			RawPreview: []byte(`{"partial":true}`),
		}
		value := readAgentHTTPResult(t.Context(), false, func(result) {}, func(publish func(agentclient.Response)) (agentclient.Response, error) {
			publish(response)
			return response, failure
		})
		if !value.done || len(value.rawPreview) != 0 || value.formattedJSON != "" || value.status == "HTTP 200 · Local Agent" {
			t.Fatalf("failed Agent response exposed success actions: %+v", value)
		}
	}
}

type interruptedResponseBody struct {
	sent bool
}

func (body *interruptedResponseBody) Read(data []byte) (int, error) {
	if body.sent {
		return 0, io.ErrUnexpectedEOF
	}
	body.sent = true
	return copy(data, "partial response"), nil
}

func (*interruptedResponseBody) Close() error { return nil }

func TestHTTPResponsePreviewReportsInterruptedBody(t *testing.T) {
	value := readHTTPResponse(context.Background(), func() (*execution.HTTPResponse, error) {
		return &execution.HTTPResponse{StatusCode: 200, Body: &interruptedResponseBody{}}, nil
	}, false, func(result) {})
	if !value.done || value.body != "partial response" ||
		!strings.Contains(value.status, "could not finish reading response") || len(value.rawPreview) != 0 || value.formattedJSON != "" {
		t.Fatalf("interrupted response appeared complete: %+v", value)
	}
}

func TestHTTPResponsePreviewCancellationDiscardsExportBytes(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	value := readHTTPResponse(ctx, func() (*execution.HTTPResponse, error) {
		return &execution.HTTPResponse{StatusCode: 200, Body: io.NopCloser(strings.NewReader("partial response"))}, nil
	}, false, func(value result) {
		if value.previewBytes > 0 {
			cancel()
		}
	})
	if !value.done || !strings.Contains(value.status, "cancelled") || len(value.rawPreview) != 0 || value.formattedJSON != "" {
		t.Fatalf("cancelled preview exposed complete-response actions: %+v", value)
	}
}
