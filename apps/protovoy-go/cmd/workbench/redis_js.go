package main

import (
	"context"
	"errors"

	"protovoy.dev/protovoy/internal/execution"
)

func exchangeRedis(ctx context.Context, payload execution.RedisPayload, options execution.RedisOptions) ([]execution.RedisValue, error) {
	payload.ExpectedPasswordRef = options.PasswordRef
	if options.PasswordRef != "" {
		if !options.TLS || options.Password != "" || options.CAPEM != "" {
			return nil, errors.New("Redis password references require configured TLS without an inline password or CA")
		}
		payload.ExpectedUsername, payload.ExpectedDatabase, payload.ExpectedServerName = options.Username, options.Database, options.ServerName
	}
	var values []execution.RedisValue
	err := fetchExecutionResponse(ctx, execution.RedisBridgePath, "Redis", payload, &values)
	return values, err
}
