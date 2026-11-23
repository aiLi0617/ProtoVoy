//go:build !js

package main

import (
	"context"
	"net/http"

	"protovoy.dev/protovoy/internal/execution"
)

func runDirectGRPC(ctx context.Context, request execution.GRPCRequest, onStatus func(int), onMessage func(execution.GRPCMessage)) error {
	return (execution.GRPCClient{}).Execute(ctx, request, func(status int, _ http.Header) error {
		onStatus(status)
		return nil
	}, func(data []byte, jsonPreview string) error {
		onMessage(execution.GRPCMessage{Size: len(data), Preview: data[:min(len(data), execution.GRPCMessagePreviewLimit)], JSONPreview: jsonPreview})
		return nil
	})
}
