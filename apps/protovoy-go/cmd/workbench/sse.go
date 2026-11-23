package main

import (
	"context"
	"fmt"
	"strings"

	"protovoy.dev/protovoy/internal/execution"
)

func readSSE(ctx context.Context, target string, payload execution.HTTPPayload, options execution.SSEOptions, open func(context.Context, string, execution.HTTPPayload) (*execution.HTTPResponse, error), publish func(result)) result {
	value := result{done: true}
	var events []string
	total := 0
	publishConnected := func() {
		publish(result{status: fmt.Sprintf("SSE connected · %d events", total), body: value.body})
	}
	reconnecting := func() {
		publish(result{status: fmt.Sprintf("SSE reconnecting · %d events", total), body: value.body})
	}
	err := execution.StreamSSE(ctx, target, payload, options, open, func(_ int) { publishConnected() }, reconnecting, func(event execution.SSEEvent) error {
		total++
		entry := event.Type
		if event.ID != "" {
			entry += " #" + event.ID
		}
		entry += "\n" + event.Data
		const maxPreview = 2048
		if len(entry) > maxPreview {
			entry = strings.ToValidUTF8(entry[:maxPreview], "�") + "…"
		}
		const visibleEvents = 20
		if len(events) == visibleEvents {
			copy(events, events[1:])
			events = events[:visibleEvents-1]
		}
		events = append(events, entry)
		value.body = strings.Join(events, "\n\n")
		publishConnected()
		return nil
	})
	switch {
	case ctx.Err() != nil:
		value.status = "SSE disconnected"
	case err != nil:
		value.status = err.Error()
	default:
		value.status = fmt.Sprintf("SSE closed · %d events", total)
	}
	return value
}
