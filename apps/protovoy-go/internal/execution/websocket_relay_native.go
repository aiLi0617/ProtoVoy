//go:build !js

package execution

import (
	"context"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// RelayWebSocket connects a bridge client to the validated upstream target.
// The caller owns the client connection and its authorization. Cancelling
// dialContext interrupts the upstream handshake without closing the client.
func RelayWebSocket(dialContext context.Context, client *websocket.Conn, target string, setup WebSocketSetup) (received int64, err error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	setupCtx, stopSetup := context.WithTimeout(ctx, 15*time.Second)
	defer stopSetup()
	upstream, err := DialWebSocket(dialContext, target, setup)
	if err != nil {
		if dialContext.Err() != nil {
			return 0, dialContext.Err()
		}
		_ = wsjson.Write(setupCtx, client, WebSocketReady{Error: err.Error()})
		return 0, err
	}
	defer upstream.CloseNow()
	if err := dialContext.Err(); err != nil {
		return 0, err
	}
	client.SetReadLimit(WebSocketMessageLimit)
	if err := wsjson.Write(setupCtx, client, WebSocketReady{Subprotocol: upstream.Subprotocol()}); err != nil {
		return 0, err
	}
	stopSetup()

	type relayEnd struct {
		fromUpstream bool
		received     int64
		err          error
	}
	done := make(chan relayEnd, 2)
	go func() {
		_, err := copyWebSocket(ctx, upstream, client)
		done <- relayEnd{err: err}
	}()
	go func() {
		received, err := copyWebSocket(ctx, client, upstream)
		done <- relayEnd{fromUpstream: true, received: received, err: err}
	}()
	first := <-done
	cancel()
	second := <-done
	if first.fromUpstream {
		return first.received, first.err
	}
	return second.received, first.err
}

func copyWebSocket(ctx context.Context, dst, src *websocket.Conn) (int64, error) {
	var received int64
	for {
		kind, data, err := src.Read(ctx)
		if err != nil {
			return received, err
		}
		received += int64(len(data))
		writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err = dst.Write(writeCtx, kind, data)
		cancel()
		if err != nil {
			return received, err
		}
	}
}
