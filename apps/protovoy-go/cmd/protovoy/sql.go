package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"time"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
	"protovoy.dev/protovoy/internal/secrets"
)

func resolveCLISQL(target string, credentials execution.SQLCredentials) (execution.SQLEndpoint, error) {
	resolve := func(string) (string, error) { return os.Getenv(execution.SQLPasswordEnv), nil }
	if os.Getenv("PROTOVOY_SQL_PASSWORD_KEYCHAIN") == "1" {
		resolve = secrets.Resolve
	}
	return execution.ResolveSQLWithPasswordResolver(target, credentials, resolve)
}

func runSQL(ctx context.Context, target, statement string, credentials execution.SQLCredentials, parameters string,
	rowLimit int, transactional bool, timeout time.Duration, output io.Writer,
) error {
	settings := project.SQLSettings{Parameters: parameters, RowLimit: rowLimit, Transactional: transactional}
	request, err := settings.Request(statement)
	if err != nil {
		return err
	}
	endpoint, err := resolveCLISQL(target, credentials)
	if err != nil {
		return err
	}
	prepared, err := endpoint.Prepare(request)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	err = prepared.Execute(ctx, func() error { return nil }, json.NewEncoder(output).Encode)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
