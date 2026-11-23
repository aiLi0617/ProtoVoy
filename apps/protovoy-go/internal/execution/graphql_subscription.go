package execution

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const GraphQLWebSocketProtocol = "graphql-transport-ws"

var ErrGraphQLSubscriptionRejected = errors.New("GraphQL subscription rejected")

func (document GraphQLDocument) PrepareSubscription(target string, headers [][2]string, scopes ...map[string]string) (WebSocketSetup, json.RawMessage, error) {
	resolved, payload, err := document.resolve(target, headers, scopes)
	if err != nil {
		return WebSocketSetup{}, nil, err
	}
	switch {
	case strings.HasPrefix(resolved, "https://"):
		resolved = "wss://" + strings.TrimPrefix(resolved, "https://")
	case strings.HasPrefix(resolved, "http://"):
		resolved = "ws://" + strings.TrimPrefix(resolved, "http://")
	}
	if (target != "" || resolved != "") && !ValidWebSocketTarget(resolved) {
		return WebSocketSetup{}, nil, errors.New("enter a valid GraphQL subscription URL")
	}
	setup := WebSocketSetup{
		ExpectedTarget: resolved, Headers: payload.Headers, Subprotocols: []string{GraphQLWebSocketProtocol},
	}
	if err := setup.Validate(); err != nil {
		return WebSocketSetup{}, nil, err
	}
	if len(payload.Body) > WebSocketMessageLimit-128 {
		return WebSocketSetup{}, nil, errors.New("GraphQL subscription exceeds the WebSocket message limit")
	}
	return setup, json.RawMessage(payload.Body), nil
}

type graphQLSocketMessage struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func readGraphQLSocketMessage(ctx context.Context, connection *websocket.Conn) (graphQLSocketMessage, error) {
	kind, data, err := connection.Read(ctx)
	if err != nil {
		return graphQLSocketMessage{}, err
	}
	var message graphQLSocketMessage
	if kind != websocket.MessageText || json.Unmarshal(data, &message) != nil || message.Type == "" {
		return graphQLSocketMessage{}, errors.New("invalid GraphQL WebSocket message")
	}
	return message, nil
}

// RunGraphQLSubscription runs one operation on an open graphql-transport-ws
// socket. The caller closes the socket after this function returns.
func RunGraphQLSubscription(ctx context.Context, connection *websocket.Conn, payload json.RawMessage, onReady func(), onMessage func(json.RawMessage, bool)) error {
	ackCtx, stopAck := context.WithTimeout(ctx, 10*time.Second)
	defer stopAck()
	if err := wsjson.Write(ackCtx, connection, graphQLSocketMessage{Type: "connection_init"}); err != nil {
		return err
	}
	for {
		message, err := readGraphQLSocketMessage(ackCtx, connection)
		if err != nil {
			return err
		}
		if message.Type == "connection_ack" {
			break
		}
		switch message.Type {
		case "ping":
			if err := wsjson.Write(ackCtx, connection, graphQLSocketMessage{Type: "pong", Payload: message.Payload}); err != nil {
				return err
			}
		case "pong":
		default:
			return errors.New("GraphQL server sent a message before acknowledging the connection")
		}
	}
	const operationID = "1"
	if err := wsjson.Write(ackCtx, connection, graphQLSocketMessage{Type: "subscribe", ID: operationID, Payload: payload}); err != nil {
		return err
	}
	stopAck()
	onReady()

	// Keep the read alive during cancellation long enough to write Complete.
	// Cancelling a coder/websocket Read can close the socket itself.
	readCtx, stopRead := context.WithCancel(context.WithoutCancel(ctx))
	defer stopRead()
	type received struct {
		message graphQLSocketMessage
		err     error
	}
	incoming := make(chan received)
	go func() {
		for {
			message, err := readGraphQLSocketMessage(readCtx, connection)
			select {
			case incoming <- received{message, err}:
			case <-readCtx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
			_ = wsjson.Write(closeCtx, connection, graphQLSocketMessage{Type: "complete", ID: operationID})
			cancel()
			return ctx.Err()
		case item := <-incoming:
			if item.err != nil {
				return item.err
			}
			message := item.message
			switch message.Type {
			case "ping":
				writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
				err := wsjson.Write(writeCtx, connection, graphQLSocketMessage{Type: "pong", Payload: message.Payload})
				cancel()
				if err != nil {
					return err
				}
			case "pong":
			case "next", "error", "complete":
				if message.ID != operationID {
					continue
				}
				if message.Type == "complete" {
					return nil
				}
				if len(message.Payload) == 0 {
					return errors.New("GraphQL result has no payload")
				}
				onMessage(message.Payload, message.Type == "error")
				if message.Type == "error" {
					return ErrGraphQLSubscriptionRejected
				}
			default:
				return errors.New("unexpected GraphQL WebSocket message")
			}
		}
	}
}
