package main

import (
	"context"
	"errors"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
	"protovoy.dev/protovoy/internal/scheduler"
)

func scheduledSQLExecutor(endpoint *sqlEndpoint) scheduler.Executor {
	return func(ctx context.Context, request project.Request) (scheduler.Outcome, error) {
		if endpoint == nil || endpoint.target == "" || request.SQL == nil || request.HasScripts() {
			return scheduler.Outcome{}, errors.New("scheduled SQL endpoint is not available for this request")
		}
		settings := request.SQL
		statement, err := settings.Request(request.Body)
		if err != nil {
			return scheduler.Outcome{}, err
		}
		input := execution.SQLBridgeRequest{ExpectedTarget: request.URL, SQLRequest: statement}
		input, err = input.ResolveVariables(request.Variables)
		if err != nil {
			return scheduler.Outcome{}, err
		}
		if input.ExpectedTarget != endpoint.target || (settings.PasswordRef != "" && settings.PasswordRef != endpoint.passwordRef) {
			return scheduler.Outcome{}, errors.New("scheduled SQL target or password reference does not match configuration")
		}
		prepared, err := endpoint.database.Prepare(input.SQLRequest)
		if err != nil {
			return scheduler.Outcome{}, err
		}
		ctx, cancel := context.WithTimeout(ctx, execution.RequestTimeout(settings.TimeoutMS))
		defer cancel()
		err = prepared.Execute(ctx, func() error { return nil }, func(any) error { return nil })
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return scheduler.Outcome{Success: err == nil}, err
	}
}
