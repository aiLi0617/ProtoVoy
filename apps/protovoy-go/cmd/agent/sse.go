package main

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"time"

	"protovoy.dev/protovoy/internal/execution"
)

func (job *agentJob) executeSSE(ctx context.Context, id, requestID string, request execution.HTTPRequest, reconnects int, delay time.Duration) agentEvent {
	started := time.Now()
	lastID := execution.SSELastEventID(request.Headers)
	var total int64
	for attempt := 0; ; attempt++ {
		if !job.emit(ctx, agentEvent{"type": "state_changed", "state": "running", "phase": "connect"}) {
			return terminalFailure(ctx, "connection")
		}
		response, err := (execution.HTTPClient{}).Execute(ctx, request)
		if err != nil {
			return terminalFailure(ctx, "connection")
		}
		contentType := response.Headers.Get("Content-Type")
		if !job.emitResponseMeta(ctx, response) {
			response.Body.Close()
			return terminalFailure(ctx, "transfer")
		}
		if !execution.IsEventStream(response.StatusCode, contentType) {
			response.Body.Close()
			return agentEvent{"type": "failed", "code": "sse_content_type", "message": "expected HTTP 200 with text/event-stream"}
		}
		writer := sseWriter(func(data []byte) (int, error) {
			if !job.emit(ctx, agentEvent{
				"type": "response_chunk", "content_type": contentType, "size": len(data), "done": false,
				"data_base64": base64.StdEncoding.EncodeToString(data), "preview": strings.ToValidUTF8(string(data), "\uFFFD"),
			}) {
				return 0, ctx.Err()
			}
			total += int64(len(data))
			return len(data), nil
		})
		decoder := execution.NewSSEDecoder(io.TeeReader(response.Body, writer), lastID, delay)
		for err == nil {
			_, err = decoder.Next()
		}
		response.Body.Close()
		if ctx.Err() != nil || !errors.Is(err, io.EOF) {
			return terminalFailure(ctx, "transfer")
		}
		if attempt >= reconnects {
			if !job.emit(ctx, agentEvent{"type": "response_chunk", "content_type": contentType, "size": 0, "done": true}) {
				return terminalFailure(ctx, "transfer")
			}
			return completedEvent(id, requestID, "sse", response.StatusCode, total, started)
		}
		lastID, delay = decoder.LastEventID, decoder.RetryDelay
		request.Headers = execution.SSEHeadersWithEventID(request.Headers, lastID)
		if !job.emit(ctx, agentEvent{"type": "warning", "code": "sse_reconnect", "message": "SSE connection closed; reconnecting"}) {
			return terminalFailure(ctx, "transfer")
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return terminalFailure(ctx, "transfer")
		case <-timer.C:
		}
	}
}

type sseWriter func([]byte) (int, error)

func (writer sseWriter) Write(data []byte) (int, error) { return writer(data) }
