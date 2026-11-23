package main

import (
	"context"
	"errors"
	"time"

	"github.com/coder/websocket"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

func executeCollectionWebSocket(ctx context.Context, request project.Request, variables map[string]string, timeout time.Duration) error {
	headers, err := execution.ParseHeaders(request.HeaderText())
	if err != nil {
		return err
	}
	options := request.WebSocket
	setup, err := execution.PrepareWebSocket(request.URL, headers, options.Subprotocols, variables, request.Variables)
	if err != nil {
		return err
	}
	frames, err := options.Frames(variables, request.Variables)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	received, err := streamWebSocket(ctx, setup, options.WebSocketAutomation, frames, func(websocket.MessageType, []byte) error { return nil })
	if err == nil && options.ReceiveLimit != nil && received < *options.ReceiveLimit {
		return errors.New("WebSocket closed before reaching the receive limit")
	}
	return err
}

func executeCollectionSSE(ctx context.Context, request project.Request, variables map[string]string, timeout time.Duration) (int, error) {
	headers, err := execution.ParseHeaders(request.HeaderText())
	if err != nil {
		return 0, err
	}
	target, payload, err := execution.PrepareSSE(request.URL, headers, variables, request.Variables)
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	status := 0
	client := execution.HTTPClient{}
	err = execution.StreamSSE(ctx, target, payload, *request.SSE,
		func(ctx context.Context, target string, payload execution.HTTPPayload) (*execution.HTTPResponse, error) {
			response, err := client.Execute(ctx, payload.Request(target))
			if response != nil {
				status = response.StatusCode
			}
			return response, err
		}, func(int) {}, func() {}, func(execution.SSEEvent) error { return nil })
	return status, err
}

func executeCollectionSubscription(ctx context.Context, request project.Request, variables map[string]string, timeout time.Duration) error {
	headers, err := execution.ParseHeaders(request.HeaderText())
	if err != nil {
		return err
	}
	setup, payload, err := request.GraphQL.PrepareSubscription(request.URL, headers, variables, request.Variables)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return streamGraphQL(ctx, setup, payload, func(data []byte, rejected bool) error {
		if rejected {
			return nil
		}
		return checkCollectionGraphQLResult(data)
	})
}
