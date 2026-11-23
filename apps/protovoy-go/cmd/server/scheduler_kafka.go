package main

import (
	"context"
	"errors"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
	"protovoy.dev/protovoy/internal/scheduler"
)

func scheduledKafkaExecutor(endpoint *kafkaEndpoint) scheduler.Executor {
	return func(ctx context.Context, request project.Request) (scheduler.Outcome, error) {
		if endpoint == nil || endpoint.target == "" || request.Kafka == nil || request.HasScripts() || !execution.ValidRequestTimeout(request.BrokerTimeoutMS) {
			return scheduler.Outcome{}, errors.New("scheduled Kafka endpoint is not available for this request")
		}
		settings := request.Kafka
		operation, err := settings.Operation(request.Body)
		if err != nil {
			return scheduler.Outcome{}, err
		}
		options := execution.KafkaOptions{KafkaOperation: operation, KafkaTLSReferences: settings.KafkaTLSReferences, PasswordRef: settings.PasswordRef, SecurityProtocol: settings.SecurityProtocol, SASLMechanism: settings.SASLMechanism, Username: settings.Username}
		target, err := options.ResolveVariables(request.URL, request.Variables)
		if err != nil {
			return scheduler.Outcome{}, err
		}
		if target != endpoint.target {
			return scheduler.Outcome{}, errors.New("scheduled Kafka target does not match configuration")
		}
		run, err := endpoint.prepare(execution.KafkaBridgeRequest{ExpectedTarget: target, KafkaOperation: options.KafkaOperation, ExpectedPasswordRef: options.PasswordRef, ExpectedTLSReferences: options.KafkaTLSReferences})
		if err != nil {
			return scheduler.Outcome{}, err
		}
		ctx, cancel := context.WithTimeout(ctx, execution.RequestTimeout(request.BrokerTimeoutMS))
		defer cancel()
		err = run(ctx, func() error { return nil }, func(any) error { return nil })
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return scheduler.Outcome{Success: err == nil}, err
	}
}
