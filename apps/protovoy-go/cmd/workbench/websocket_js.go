package main

import (
	"context"
	"encoding/json"
	"errors"
	"syscall/js"
	"time"

	"github.com/coder/websocket"
	"protovoy.dev/protovoy/internal/execution"
)

func connectWebSocket(ctx context.Context, setup execution.WebSocketSetup) (*websocket.Conn, string, error) {
	data, err := json.Marshal(setup)
	if err != nil || len(data) > execution.WebSocketSetupLimit {
		return nil, "", errors.New("WebSocket setup exceeds 64 KiB")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	origin := js.Global().Get("location").Get("origin").String()
	connection, _, err := websocket.Dial(ctx, origin+execution.WebSocketBridgePath, nil)
	if err != nil {
		return nil, "", errors.New("could not open the WebSocket bridge")
	}
	if err := connection.Write(ctx, websocket.MessageText, data); err != nil {
		closeWorkbenchSocket(connection)
		return nil, "", errors.New("could not send WebSocket setup")
	}
	return awaitSocketReady(ctx, connection)
}

func dialAgentSocket(ctx context.Context, endpoint string, protocols []string) (*websocket.Conn, error) {
	connection, _, err := websocket.Dial(ctx, endpoint, &websocket.DialOptions{Subprotocols: protocols})
	return connection, err
}

func closeWorkbenchSocket(connection *websocket.Conn) {
	// The browser close API accepts 1000 or application-defined codes only.
	// coder/websocket CloseNow uses 1001 on its WASM backend.
	_ = connection.Close(websocket.StatusNormalClosure, "")
}
