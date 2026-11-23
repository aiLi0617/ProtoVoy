//go:build !js

package execution

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

func DialWebSocket(ctx context.Context, target string, setup WebSocketSetup) (*websocket.Conn, error) {
	if !ValidWebSocketTarget(target) {
		return nil, errors.New("enter a valid ws:// or wss:// URL")
	}
	if err := setup.Validate(); err != nil {
		return nil, err
	}
	headers := make(http.Header)
	for _, pair := range setup.Headers {
		headers.Add(pair[0], pair[1])
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	connection, _, err := websocket.Dial(ctx, target, &websocket.DialOptions{
		HTTPHeader: headers, Subprotocols: setup.Subprotocols,
		HTTPClient: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
	})
	if err != nil {
		return nil, errors.New("WebSocket handshake failed")
	}
	connection.SetReadLimit(WebSocketMessageLimit)
	return connection, nil
}
