package main

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"protovoy.dev/protovoy/internal/execution"
)

func runGraphQL(ctx context.Context, target string, document execution.GraphQLDocument, headers [][2]string, timeout time.Duration, output, status io.Writer) error {
	if !document.Subscription {
		endpoint, payload, err := document.Prepare(target, headers)
		if err != nil {
			return err
		}
		request := payload.Request(endpoint)
		request.Timeout = timeout
		return run(ctx, request, output, status)
	}

	setup, payload, err := document.PrepareSubscription(target, headers)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	encoder := json.NewEncoder(output)
	return streamGraphQL(ctx, setup, payload, func(data []byte, _ bool) error {
		return encoder.Encode(json.RawMessage(data))
	})
}

func streamGraphQL(ctx context.Context, setup execution.WebSocketSetup, payload json.RawMessage, receive func([]byte, bool) error) error {
	return execution.StreamGraphQL(ctx, setup, payload, receive)
}
