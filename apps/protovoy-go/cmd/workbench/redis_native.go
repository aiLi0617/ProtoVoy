//go:build !js

package main

import (
	"context"
	"os"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/secrets"
)

func exchangeRedis(ctx context.Context, payload execution.RedisPayload, options execution.RedisOptions) ([]execution.RedisValue, error) {
	options.Protocol = payload.Protocol
	options, err := prepareDirectRedisOptions(payload.ExpectedTarget, options)
	if err != nil {
		return nil, err
	}
	return execution.ExecuteRedisWithOptions(ctx, payload.ExpectedTarget, payload.Commands, execution.RequestTimeout(payload.TimeoutMS), options)
}

func prepareDirectRedisOptions(target string, options execution.RedisOptions) (execution.RedisOptions, error) {
	resolve := func(string) (string, error) { return os.Getenv(execution.RedisPasswordEnv), nil }
	if os.Getenv("PROTOVOY_REDIS_PASSWORD_KEYCHAIN") == "1" {
		resolve = secrets.Resolve
	}
	return execution.ResolveRedisOptions(target, options, resolve)
}
