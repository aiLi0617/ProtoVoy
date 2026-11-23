package main

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
	"protovoy.dev/protovoy/internal/scheduler"
)

func scheduledGRPCExecutor(target string) scheduler.Executor {
	return func(ctx context.Context, request project.Request) (scheduler.Outcome, error) {
		if target == "" || request.GRPC == nil || request.HasScripts() || !execution.ValidRequestTimeout(request.GRPC.TimeoutMS) {
			return scheduler.Outcome{}, errors.New("scheduled gRPC endpoint is not available for this request")
		}
		settings := request.GRPC
		metadata, err := execution.ParseHeaders(request.HeaderText())
		if err != nil {
			return scheduler.Outcome{}, err
		}
		input := execution.GRPCRequest{Target: request.URL, Service: settings.Service, Method: settings.Method, Mode: settings.Mode,
			Metadata: metadata, DescriptorSetBase64: settings.DescriptorSetBase64, TimeoutMS: settings.TimeoutMS, MessageBase64: request.Body}
		if settings.UseJSON {
			input.MessageBase64, input.MessageJSON = "", request.Body
		}
		input, err = input.ResolveVariables(request.Variables)
		if err != nil {
			return scheduler.Outcome{}, err
		}
		if input.Target != target {
			return scheduler.Outcome{}, errors.New("scheduled gRPC target does not match configuration")
		}
		if settings.UseJSON && strings.TrimSpace(input.MessageJSON) == "" {
			return scheduler.Outcome{}, errors.New("enter a gRPC JSON message")
		}
		status := 0
		err = (execution.GRPCClient{}).Execute(ctx, input, func(code int, _ http.Header) error { status = code; return nil }, func([]byte, string) error { return nil })
		passed := err == nil && status >= 200 && status < 300
		if request.ExpectedHTTPStatus != 0 {
			passed = err == nil && status == request.ExpectedHTTPStatus
		}
		return scheduler.Outcome{Success: passed, Status: status}, err
	}
}
