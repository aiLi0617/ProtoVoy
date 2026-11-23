package main

import (
	"context"
	"errors"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
	"protovoy.dev/protovoy/internal/scheduler"
)

func scheduledSSEExecutor(target string, additional map[string]struct{}) scheduler.Executor {
	allowed := map[string]bool{}
	if target != "" {
		allowed[target] = true
	}
	for value := range additional {
		allowed[value] = true
	}
	return func(ctx context.Context, request project.Request) (scheduler.Outcome, error) {
		if request.SSE == nil || !request.SSE.Valid() || request.HasScripts() || request.HTTPSecretReference != "" || !execution.ValidRequestTimeout(request.HTTPTimeoutMS) {
			return scheduler.Outcome{}, errors.New("scheduled SSE endpoint is not available for this request")
		}
		headers, err := execution.ParseHeaders(request.HeaderText())
		if err != nil {
			return scheduler.Outcome{}, err
		}
		resolved, payload, err := execution.PrepareSSE(request.URL, headers, request.Variables)
		if err != nil {
			return scheduler.Outcome{}, err
		}
		if !allowed[resolved] {
			return scheduler.Outcome{}, errors.New("scheduled SSE target does not match configuration")
		}
		ctx, cancel := context.WithTimeout(ctx, execution.RequestTimeout(request.HTTPTimeoutMS))
		defer cancel()
		status := 0
		err = execution.StreamSSE(ctx, resolved, payload, *request.SSE,
			func(ctx context.Context, current string, payload execution.HTTPPayload) (*execution.HTTPResponse, error) {
				if !allowed[current] || payload.FollowRedirects {
					return nil, errors.New("scheduled SSE target does not match configuration")
				}
				response, err := (execution.HTTPClient{}).Execute(ctx, payload.Request(current))
				if response != nil {
					status = response.StatusCode
				}
				return response, err
			}, func(int) {}, func() {}, func(execution.SSEEvent) error { return nil })
		passed := err == nil
		if request.ExpectedHTTPStatus != 0 {
			passed = passed && status == request.ExpectedHTTPStatus
		}
		return scheduler.Outcome{Success: passed, Status: status}, err
	}
}
