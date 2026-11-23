package main

import (
	"context"
	"errors"

	"github.com/coder/websocket"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
	"protovoy.dev/protovoy/internal/scheduler"
)

func scheduledWebSocketExecutor(target string) scheduler.Executor {
	return func(ctx context.Context, request project.Request) (scheduler.Outcome, error) {
		if target == "" || request.WebSocket == nil || request.HasScripts() {
			return scheduler.Outcome{}, errors.New("scheduled WebSocket endpoint is not available for this request")
		}
		headers, err := execution.ParseHeaders(request.HeaderText())
		if err != nil {
			return scheduler.Outcome{}, err
		}
		options := request.WebSocket
		setup, err := execution.PrepareWebSocket(request.URL, headers, options.Subprotocols, request.Variables)
		if err != nil {
			return scheduler.Outcome{}, err
		}
		if setup.ExpectedTarget != target {
			return scheduler.Outcome{}, errors.New("scheduled WebSocket target does not match configuration")
		}
		frames, err := options.Frames(request.Variables)
		if err != nil {
			return scheduler.Outcome{}, err
		}
		received, err := execution.StreamWebSocket(ctx, setup, options.WebSocketAutomation, frames, func(websocket.MessageType, []byte) error { return nil })
		if err == nil && options.ReceiveLimit != nil && received < *options.ReceiveLimit {
			err = errors.New("WebSocket closed before reaching the receive limit")
		}
		return scheduler.Outcome{Success: err == nil}, err
	}
}
