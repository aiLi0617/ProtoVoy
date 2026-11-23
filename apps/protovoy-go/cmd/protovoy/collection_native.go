package main

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

func executeCollectionGRPC(ctx context.Context, request project.Request, variables map[string]string) (int, error) {
	settings := request.GRPC
	metadata, err := execution.ParseHeaders(request.HeaderText())
	if err != nil {
		return 0, err
	}
	input := execution.GRPCRequest{
		Target: request.URL, Service: settings.Service, Method: settings.Method, Mode: settings.Mode,
		Metadata: metadata, DescriptorSetBase64: settings.DescriptorSetBase64, TimeoutMS: settings.TimeoutMS,
		MessageBase64: request.Body,
	}
	if settings.UseJSON {
		input.MessageBase64, input.MessageJSON = "", request.Body
	}
	input, err = input.ResolveVariables(variables, request.Variables)
	if err != nil {
		return 0, err
	}
	if settings.UseJSON && strings.TrimSpace(input.MessageJSON) == "" {
		return 0, errors.New("enter a gRPC JSON message")
	}
	status := 0
	err = (execution.GRPCClient{}).Execute(ctx, input, func(code int, _ http.Header) error {
		status = code
		return nil
	}, func([]byte, string) error { return nil })
	return status, err
}

func executeCollectionSocket(ctx context.Context, request project.Request, variables map[string]string) error {
	payload := *request.Socket
	payload.ExpectedTarget = request.URL
	payload, err := payload.ResolveVariables(variables, request.Variables)
	if err != nil {
		return err
	}
	_, err = execution.ExchangeSocket(ctx, payload.Request(payload.ExpectedTarget))
	return err
}

func executeCollectionSQL(ctx context.Context, request project.Request, variables map[string]string) error {
	settings := request.SQL
	statement, err := settings.Request(request.Body)
	if err != nil {
		return err
	}
	input := execution.SQLBridgeRequest{ExpectedTarget: request.URL, SQLRequest: statement}
	input, err = input.ResolveVariables(variables, request.Variables)
	if err != nil {
		return err
	}
	credentials := execution.SQLCredentials{Username: settings.Username, PasswordRef: settings.PasswordRef}
	endpoint, err := resolveCLISQL(input.ExpectedTarget, credentials)
	if err != nil {
		return err
	}
	prepared, err := endpoint.Prepare(input.SQLRequest)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, execution.RequestTimeout(settings.TimeoutMS))
	defer cancel()
	err = prepared.Execute(ctx, func() error { return nil }, func(any) error { return nil })
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func executeCollectionRedis(ctx context.Context, request project.Request, variables map[string]string) error {
	settings := request.Redis
	commands, err := execution.ParseRedisCommands(request.Body)
	if err != nil {
		return err
	}
	payload := execution.RedisPayload{ExpectedTarget: request.URL, Commands: commands}
	if err := payload.ResolveVariables(variables, request.Variables); err != nil {
		return err
	}
	values, err := executeRedis(ctx, payload.ExpectedTarget, payload.Commands,
		execution.RequestTimeout(request.BrokerTimeoutMS), execution.RedisOptions{
			Protocol: settings.Protocol, Username: settings.Username, Database: settings.Database,
			PasswordRef: settings.PasswordRef,
			TLS:         settings.TLS, ServerName: settings.ServerName, CAPEM: settings.CAPEM,
		})
	if err != nil {
		return err
	}
	for _, value := range values {
		if value.Kind == "error" {
			return errors.New("Redis command returned an error")
		}
	}
	return nil
}
