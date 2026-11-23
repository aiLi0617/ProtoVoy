package agentclient

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"

	"protovoy.dev/protovoy/internal/execution"
)

func (client *Client) OpenSSE(ctx context.Context, target string, payload execution.HTTPPayload) (*execution.HTTPResponse, error) {
	// The workbench reconnects using the raw upstream stream, so each Agent
	// execution represents one HTTP connection.
	stream, err := client.openExecution(ctx, target, "sse", map[string]any{
		"type": "sse", "headers": append([][2]string{}, payload.Headers...), "reconnectMax": 0,
	})
	if err != nil {
		return nil, err
	}
	body := &sseBody{executionStream: stream, decoder: execution.NewSSEDecoder(stream.body, "", 0)}
	for {
		event, err := nextAgentEvent(body.decoder)
		if err != nil {
			body.Close()
			return nil, err
		}
		switch event.Type {
		case "response_meta":
			headers := make(http.Header)
			headers.Set("Content-Type", event.ContentType)
			return &execution.HTTPResponse{StatusCode: event.Status, Headers: headers, Body: body}, nil
		case "failed", "cancelled", "completed":
			body.finished = true
			body.Close()
			if event.Type == "cancelled" {
				return nil, context.Canceled
			}
			return nil, errors.New("Local Agent could not open the SSE stream")
		}
	}
}

type sseBody struct {
	*executionStream
	decoder *execution.SSEDecoder
	pending []byte
}

func (body *sseBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for len(body.pending) == 0 {
		if body.finished {
			return 0, io.EOF
		}
		event, err := nextAgentEvent(body.decoder)
		if err != nil {
			return 0, err
		}
		switch event.Type {
		case "response_chunk":
			if event.DataBase64 != "" {
				body.pending, err = base64.StdEncoding.DecodeString(event.DataBase64)
				if err != nil {
					return 0, errors.New("Local Agent returned invalid SSE bytes")
				}
			} else if event.Preview != nil && !event.Done {
				body.pending = []byte(*event.Preview)
			}
		case "completed":
			body.finished = true
			return 0, io.EOF
		case "cancelled":
			body.finished = true
			return 0, context.Canceled
		case "failed":
			body.finished = true
			return 0, errors.New("Local Agent SSE execution failed")
		}
	}
	n := copy(p, body.pending)
	body.pending = body.pending[n:]
	return n, nil
}
