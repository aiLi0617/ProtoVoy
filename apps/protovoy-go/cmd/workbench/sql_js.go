//go:build js

package main

import (
	"context"
	"encoding/json"

	"protovoy.dev/protovoy/internal/execution"
)

func runDirectSQL(ctx context.Context, input execution.SQLBridgeRequest, credentials execution.SQLCredentials, connected func(), message func(json.RawMessage)) error {
	input.ExpectedPasswordRef = credentials.PasswordRef
	return runBrokerBridge(ctx, execution.SQLBridgePath, "SQL", input, connected, message)
}
