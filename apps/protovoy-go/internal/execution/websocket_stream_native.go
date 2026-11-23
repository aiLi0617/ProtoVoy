//go:build !js

package execution

import (
	"context"
	"errors"
	"fmt"
	"github.com/coder/websocket"
	"time"
)

func StreamWebSocket(ctx context.Context, setup WebSocketSetup, options WebSocketAutomation, frames [][]byte, receive func(websocket.MessageType, []byte) error) (int, error) {
	received := 0
	for attempt := 0; ; attempt++ {
		count, retry, err := webSocketSession(ctx, setup, options, frames, received, receive)
		received = count
		if !retry || attempt >= options.ReconnectMax {
			return received, err
		}
		timer := time.NewTimer(options.ReconnectDelay())
		select {
		case <-ctx.Done():
			timer.Stop()
			return received, ctx.Err()
		case <-timer.C:
		}
	}
}

func webSocketSession(ctx context.Context, setup WebSocketSetup, options WebSocketAutomation, frames [][]byte,
	received int, receive func(websocket.MessageType, []byte) error,
) (int, bool, error) {
	conn, err := DialWebSocket(ctx, setup.ExpectedTarget, setup)
	if err != nil {
		if ctx.Err() != nil {
			return received, false, ctx.Err()
		}
		return received, true, err
	}
	defer conn.CloseNow()
	for i, frame := range frames {
		kind := websocket.MessageText
		if options.Messages[i].Encoding == "binary" {
			kind = websocket.MessageBinary
		}
		if err := conn.Write(ctx, kind, frame); err != nil {
			if ctx.Err() != nil {
				return received, false, ctx.Err()
			}
			return received, false, errors.New("could not send WebSocket message")
		}
	}

	for options.ReceiveLimit == nil || received < *options.ReceiveLimit {
		readContext, stopRead := ctx, func() {}
		if options.IdleTimeoutMS > 0 {
			readContext, stopRead = context.WithTimeout(ctx, time.Duration(options.IdleTimeoutMS)*time.Millisecond)
		}
		kind, data, err := conn.Read(readContext)
		stopRead()
		if err != nil {
			if ctx.Err() != nil {
				return received, false, ctx.Err()
			}
			switch code := websocket.CloseStatus(err); code {
			case websocket.StatusNormalClosure, websocket.StatusGoingAway:
				return received, true, nil
			case -1:
				if errors.Is(err, context.DeadlineExceeded) {
					return received, true, errors.New("WebSocket idle timeout")
				}
				return received, true, errors.New("could not read WebSocket message")
			default:
				return received, true, fmt.Errorf("WebSocket closed with status %d", code)
			}
		}
		if err := receive(kind, data); err != nil {
			return received, false, err
		}
		received++
	}
	return received, false, nil
}
