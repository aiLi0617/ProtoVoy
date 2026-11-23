//go:build !js

package execution

import (
	"context"
	"encoding/json"
	"errors"
)

func StreamGraphQL(ctx context.Context, setup WebSocketSetup, payload json.RawMessage, receive func([]byte, bool) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	connection, err := DialWebSocket(ctx, setup.ExpectedTarget, setup)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	defer connection.CloseNow()
	if connection.Subprotocol() != GraphQLWebSocketProtocol {
		return errors.New("server did not negotiate graphql-transport-ws")
	}
	var outputErr error
	err = RunGraphQLSubscription(ctx, connection, payload, func() {}, func(data json.RawMessage, rejected bool) {
		if outputErr == nil {
			outputErr = receive(data, rejected)
			if outputErr != nil {
				cancel()
			}
		}
	})
	if outputErr != nil {
		return outputErr
	}
	return err
}
