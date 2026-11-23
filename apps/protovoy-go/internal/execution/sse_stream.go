package execution

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

var errSSERead = errors.New("SSE stream could not be read (event limit: 64 KiB)")

// StreamSSE delivers events as they arrive. A clean disconnect can reconnect
// with the latest Last-Event-ID and the retry delay supplied by the server.
func StreamSSE(ctx context.Context, target string, payload HTTPPayload, options SSEOptions,
	open func(context.Context, string, HTTPPayload) (*HTTPResponse, error),
	connected func(status int), reconnecting func(), receive func(SSEEvent) error,
) error {
	lastID, delay := SSELastEventID(payload.Headers), options.InitialRetryDelay()
	for attempt := 0; ; attempt++ {
		response, err := open(ctx, target, payload)
		if ctx.Err() != nil {
			if response != nil {
				response.Body.Close()
			}
			return ctx.Err()
		}
		if err != nil {
			return err
		}
		if !IsEventStream(response.StatusCode, response.Headers.Get("Content-Type")) {
			response.Body.Close()
			return fmt.Errorf("HTTP %d · expected text/event-stream", response.StatusCode)
		}
		connected(response.StatusCode)
		decoder := NewSSEDecoder(response.Body, lastID, delay)
		for {
			event, readErr := decoder.Next()
			if readErr != nil {
				err = readErr
				break
			}
			if err := receive(event); err != nil {
				response.Body.Close()
				return err
			}
		}
		response.Body.Close()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !errors.Is(err, io.EOF) {
			return errSSERead
		}
		if attempt >= options.Reconnects {
			return nil
		}
		lastID, delay = decoder.LastEventID, decoder.RetryDelay
		payload.Headers = SSEHeadersWithEventID(payload.Headers, lastID)
		reconnecting()
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
