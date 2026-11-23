//go:build !js

package main

import (
	"context"
	"protovoy.dev/protovoy/internal/execution"
)

func exchangeDatagram(ctx context.Context, payload execution.SocketPayload) ([]byte, error) {
	return execution.ExchangeSocket(ctx, payload.Request(payload.ExpectedTarget))
}
