package main

import (
	"context"
	"errors"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
	"protovoy.dev/protovoy/internal/scheduler"
)

func scheduledAMQPExecutor(endpoint *amqpEndpoint) scheduler.Executor {
	return func(ctx context.Context, request project.Request) (scheduler.Outcome, error) {
		if endpoint == nil || endpoint.target == "" || request.AMQP == nil || request.HasScripts() || !execution.ValidRequestTimeout(request.BrokerTimeoutMS) {
			return scheduler.Outcome{}, errors.New("scheduled AMQP endpoint is not available for this request")
		}
		settings := request.AMQP
		options := execution.AMQPOptions{Mode: settings.Mode, Exchange: settings.Exchange, ExchangeType: settings.ExchangeType, RoutingKey: settings.RoutingKey, Queue: settings.Queue, Encoding: settings.Encoding, ContentType: settings.ContentType, Declare: settings.Declare, Durable: settings.Durable, AutoAck: settings.AutoAck, ReceiveLimit: settings.ReceiveLimit, PasswordRef: settings.PasswordRef}
		if options.Mode == "publish" {
			options.Payload = request.Body
		}
		target, err := options.ResolveVariables(request.URL, request.Variables)
		if err != nil {
			return scheduler.Outcome{}, err
		}
		if target != endpoint.target {
			return scheduler.Outcome{}, errors.New("scheduled AMQP target does not match configuration")
		}
		run, err := endpoint.prepare(execution.AMQPBridgeRequest{ExpectedTarget: target, ExpectedPasswordRef: options.PasswordRef, Mode: options.Mode, Exchange: options.Exchange, ExchangeType: options.ExchangeType, RoutingKey: options.RoutingKey, Queue: options.Queue, Payload: options.Payload, Encoding: options.Encoding, ContentType: options.ContentType, Declare: &options.Declare, Durable: options.Durable, AutoAck: options.AutoAck, ReceiveLimit: options.ReceiveLimit})
		if err != nil {
			return scheduler.Outcome{}, err
		}
		ctx, cancel := context.WithTimeout(ctx, execution.RequestTimeout(request.BrokerTimeoutMS))
		defer cancel()
		confirmed := options.Mode != "publish"
		err = run(ctx, func() error { return nil }, func(result any) error {
			if options.Mode != "publish" {
				return nil
			}
			confirmation, ok := result.(map[string]any)
			if !ok || confirmation["published"] != true {
				return errors.New("AMQP publication was not confirmed or was returned")
			}
			confirmed = true
			return nil
		})
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		if err == nil && !confirmed {
			err = errors.New("AMQP publication was not confirmed")
		}
		return scheduler.Outcome{Success: err == nil}, err
	}
}
