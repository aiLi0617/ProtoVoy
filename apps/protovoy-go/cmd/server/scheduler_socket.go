package main

import (
	"context"
	"errors"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
	"protovoy.dev/protovoy/internal/scheduler"
)

func scheduledSocketExecutor(tcpTarget, udpTarget string) scheduler.Executor {
	return func(ctx context.Context, request project.Request) (scheduler.Outcome, error) {
		if request.Socket == nil || request.HasScripts() {
			return scheduler.Outcome{}, errors.New("request requires another scheduled execution adapter")
		}
		input := *request.Socket
		input.ExpectedTarget = request.URL
		input, err := input.ResolveVariables(request.Variables)
		if err != nil {
			return scheduler.Outcome{}, err
		}
		var target string
		switch input.Network {
		case "tcp":
			target = tcpTarget
		case "udp":
			target = udpTarget
		default:
			return scheduler.Outcome{}, errors.New("select TCP or UDP")
		}
		if target == "" || input.ExpectedTarget != target {
			return scheduler.Outcome{}, errors.New("scheduled socket target does not match a configured endpoint")
		}
		if !input.ValidSendOptions() || len(input.Data) > (8<<20)/max(input.SendCount, 1) || !execution.ValidRequestTimeout(input.TimeoutMS) {
			return scheduler.Outcome{}, errors.New("invalid scheduled socket send options")
		}
		_, err = execution.ExchangeSocket(ctx, input.Request(target))
		return scheduler.Outcome{Success: err == nil}, err
	}
}
