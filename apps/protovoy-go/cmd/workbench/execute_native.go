//go:build !js

package main

import (
	"context"

	"protovoy.dev/protovoy/internal/execution"
)

const localTarget = true

func execute(ctx context.Context, target string, payload execution.HTTPPayload) (*execution.HTTPResponse, error) {
	payload, err := payload.EncodeBody()
	if err != nil {
		return nil, err
	}
	return (execution.HTTPClient{}).Execute(ctx, payload.Request(target))
}
