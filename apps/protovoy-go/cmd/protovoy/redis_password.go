package main

import (
	"context"
	"os"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/secrets"
	"time"
)

func executeRedis(ctx context.Context, target string, commands [][]string, timeout time.Duration, options execution.RedisOptions) ([]execution.RedisValue, error) {
	resolve := func(string) (string, error) { return os.Getenv(execution.RedisPasswordEnv), nil }
	if os.Getenv("PROTOVOY_REDIS_PASSWORD_KEYCHAIN") == "1" {
		resolve = secrets.Resolve
	}
	return execution.ExecuteRedisWithPasswordResolver(ctx, target, commands, timeout, options, resolve)
}
