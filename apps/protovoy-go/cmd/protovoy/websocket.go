package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"time"

	"github.com/coder/websocket"
	"protovoy.dev/protovoy/internal/execution"
)

func runWebSocket(ctx context.Context, target string, headers [][2]string, protocols string,
	options execution.WebSocketAutomation, timeout time.Duration, output io.Writer,
) error {
	setup, err := execution.PrepareWebSocket(target, headers, protocols)
	if err != nil {
		return err
	}
	frames, err := options.Frames()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	encoder := json.NewEncoder(output)
	_, err = streamWebSocket(ctx, setup, options, frames, func(kind websocket.MessageType, data []byte) error {
		message := execution.WebSocketMessage{Encoding: "text", Data: string(data)}
		if kind == websocket.MessageBinary {
			message.Encoding = "binary"
			message.Data = base64.StdEncoding.EncodeToString(data)
		}
		return encoder.Encode(message)
	})
	return err
}

func streamWebSocket(ctx context.Context, setup execution.WebSocketSetup, options execution.WebSocketAutomation, frames [][]byte, receive func(websocket.MessageType, []byte) error) (int, error) {
	return execution.StreamWebSocket(ctx, setup, options, frames, receive)
}
