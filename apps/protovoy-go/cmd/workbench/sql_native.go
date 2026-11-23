//go:build !js

package main

import (
	"context"
	"encoding/json"
	"os"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/secrets"
)

func runDirectSQL(ctx context.Context, input execution.SQLBridgeRequest, credentials execution.SQLCredentials, connected func(), message func(json.RawMessage)) error {
	endpoint, err := resolveDirectSQL(input.ExpectedTarget, credentials)
	if err != nil {
		return err
	}
	request, err := endpoint.Prepare(input.SQLRequest)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, execution.RequestTimeout(input.TimeoutMS))
	defer cancel()
	err = request.Execute(ctx, func() error { connected(); return nil }, func(value any) error {
		message(value.(json.RawMessage))
		return nil
	})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func resolveDirectSQL(target string, credentials execution.SQLCredentials) (execution.SQLEndpoint, error) {
	resolve := func(string) (string, error) { return os.Getenv(execution.SQLPasswordEnv), nil }
	if os.Getenv("PROTOVOY_SQL_PASSWORD_KEYCHAIN") == "1" {
		resolve = secrets.Resolve
	}
	return execution.ResolveSQLWithPasswordResolver(target, credentials, resolve)
}
