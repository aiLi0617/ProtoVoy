package main

import (
	"context"
	"errors"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
	"protovoy.dev/protovoy/internal/scheduler"
)

func scheduledGraphQLExecutor(target string) scheduler.Executor {
	return func(ctx context.Context, request project.Request) (scheduler.Outcome, error) {
		if target == "" || !request.IsGraphQLSubscription() || request.HasScripts() {
			return scheduler.Outcome{}, errors.New("scheduled GraphQL subscription endpoint is not available for this request")
		}
		headers, err := execution.ParseHeaders(request.HeaderText())
		if err != nil {
			return scheduler.Outcome{}, err
		}
		setup, payload, err := request.GraphQL.PrepareSubscription(request.URL, headers, request.Variables)
		if err != nil {
			return scheduler.Outcome{}, err
		}
		if setup.ExpectedTarget != target {
			return scheduler.Outcome{}, errors.New("scheduled GraphQL subscription target does not match configuration")
		}
		err = execution.StreamGraphQL(ctx, setup, payload, func(data []byte, rejected bool) error {
			if rejected {
				return execution.ErrGraphQLSubscriptionRejected
			}
			return execution.CheckCollectionGraphQLResult(data)
		})
		return scheduler.Outcome{Success: err == nil}, err
	}
}
