package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"protovoy.dev/protovoy/internal/execution"
)

func runSSE(ctx context.Context, target string, headers [][2]string, reconnects, delay string, timeout time.Duration, output, status io.Writer) error {
	options, err := execution.ParseSSEOptions(reconnects, delay)
	if err != nil {
		return err
	}
	target, payload, err := execution.PrepareSSE(target, headers)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	client := execution.HTTPClient{}
	encoder := json.NewEncoder(output)
	return execution.StreamSSE(ctx, target, payload, options,
		func(ctx context.Context, target string, payload execution.HTTPPayload) (*execution.HTTPResponse, error) {
			return client.Execute(ctx, payload.Request(target))
		},
		func(code int) { fmt.Fprintf(status, "HTTP %d\n", code) },
		func() {},
		func(event execution.SSEEvent) error {
			return encoder.Encode(struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Data string `json:"data"`
			}{event.Type, event.ID, event.Data})
		},
	)
}
