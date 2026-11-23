package main

import (
	"bytes"
	"context"
	"errors"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
	"protovoy.dev/protovoy/internal/scheduler"
	"protovoy.dev/protovoy/internal/scripting"
	"protovoy.dev/protovoy/internal/secrets"
)

// scheduledHTTPExecutor is the HTTP-family adapter. Other protocol adapters
// must be selected by the dispatcher rather than silently treated as HTTP.
func scheduledHTTPExecutor(target string, additional map[string]struct{}, keychain ...bool) scheduler.Executor {
	allowCredentials := len(keychain) == 1 && keychain[0]
	allowed := map[string]bool{}
	if target != "" {
		allowed[target] = true
	}
	for value := range additional {
		allowed[value] = true
	}
	return func(ctx context.Context, request project.Request) (scheduler.Outcome, error) {
		protocol := request.CollectionHTTPProtocol()
		if protocol == "" || (request.HasScripts() && protocol != "http") {
			return scheduler.Outcome{}, errors.New("request requires another scheduled execution adapter")
		}
		if request.HTTPSecretReference != "" && (!allowCredentials || request.HasScripts() || protocol != "http") {
			return scheduler.Outcome{}, errors.New("scheduled HTTP credential access is disabled or incompatible with this request")
		}
		input, err := request.PrepareCollectionHTTP(nil)
		if err != nil {
			return scheduler.Outcome{}, err
		}
		if !allowed[input.Target] || !validHostedHTTPMethod(input.Method) || input.FollowRedirects {
			return scheduler.Outcome{}, errors.New("scheduled HTTP request does not match hosted execution policy")
		}
		expectedBody := request.ResponseBodyContains
		if expectedBody != "" {
			expectedBody, err = execution.ResolveTextVariables(expectedBody, request.Variables)
			if err != nil || len(expectedBody) == 0 || len(expectedBody) > project.MaxResponseBodyAssertionBytes {
				return scheduler.Outcome{}, errors.New("invalid scheduled response assertion")
			}
		}
		if request.HTTPSecretReference != "" {
			input.Headers, err = secrets.HTTPHeadersContext(ctx, input.Headers, input.Target, request.HTTPSecretReference, request.HTTPSecretHeader, input.FollowRedirects)
			if err != nil {
				return scheduler.Outcome{}, errors.New("could not apply scheduled HTTP credential binding")
			}
		}
		status, err := executeScheduledHTTP(ctx, request, input, protocol, expectedBody, allowed)
		passed := err == nil && status >= 200 && status < 300
		if request.ExpectedHTTPStatus != 0 {
			passed = err == nil && status == request.ExpectedHTTPStatus
		}
		return scheduler.Outcome{Success: passed, Status: status}, err
	}
}

func executeScheduledHTTP(ctx context.Context, request project.Request, input execution.HTTPRequest, protocol, expectedBody string, allowed map[string]bool) (int, error) {
	if !request.HasScripts() {
		return execution.CollectHTTPStatus(ctx, execution.HTTPClient{}, input, protocol, expectedBody)
	}
	input, data, variables, err := scripting.PrepareHTTP(ctx, input, request.PreScripts, request.Variables)
	if err != nil {
		return 0, errors.New("scheduled pre-request script failed")
	}
	if !allowed[input.Target] || !validHostedHTTPMethod(input.Method) || input.FollowRedirects {
		return 0, errors.New("scheduled script request does not match hosted execution policy")
	}
	if len(request.PostScripts) == 0 {
		return execution.CollectHTTPStatus(ctx, execution.HTTPClient{}, input, protocol, expectedBody)
	}
	status, body, err := scripting.ExecuteHTTPPost(ctx, execution.HTTPClient{}, input, request.PostScripts, data, variables)
	if err != nil {
		return status, errors.New("scheduled post-response script failed")
	}
	if expectedBody != "" && !bytes.Contains(body, []byte(expectedBody)) {
		return status, errors.New("response body does not contain the expected text")
	}
	return status, nil
}
