//go:build !js

package main

import (
	"context"

	"github.com/coder/websocket"
	"protovoy.dev/protovoy/internal/execution"
)

func connectWebSocket(ctx context.Context, setup execution.WebSocketSetup) (*websocket.Conn, string, error) {
	connection, err := execution.DialWebSocket(ctx, setup.ExpectedTarget, setup)
	if err != nil {
		return nil, "", err
	}
	return connection, connection.Subprotocol(), nil
}

func closeWorkbenchSocket(connection *websocket.Conn) {
	_ = connection.CloseNow()
}

func dialAgentSocket(ctx context.Context, endpoint string, protocols []string) (*websocket.Conn, error) {
	connection, _, err := websocket.Dial(ctx, endpoint, &websocket.DialOptions{HTTPClient: agentHTTP, Subprotocols: protocols})
	return connection, err
}
