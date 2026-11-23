package main

import (
	"context"
	"errors"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
	"protovoy.dev/protovoy/internal/scheduler"
)

func scheduledMQTTExecutor(endpoint *mqttEndpoint) scheduler.Executor {
	return func(ctx context.Context, request project.Request) (scheduler.Outcome, error) {
		if endpoint == nil || endpoint.target == "" || request.MQTT == nil || request.HasScripts() || !execution.ValidRequestTimeout(request.BrokerTimeoutMS) {
			return scheduler.Outcome{}, errors.New("scheduled MQTT endpoint is not available for this request")
		}
		settings := request.MQTT
		options := execution.MQTTOptions{Mode: settings.Mode, Topic: settings.Topic, Encoding: settings.Encoding, QoS: settings.QoS, Retain: settings.Retain, ReceiveLimit: settings.ReceiveLimit, PasswordRef: settings.PasswordRef}
		if options.Mode == "publish" {
			options.Payload = request.Body
		}
		target, err := options.ResolveVariables(request.URL, request.Variables)
		if err != nil {
			return scheduler.Outcome{}, err
		}
		if target != endpoint.target {
			return scheduler.Outcome{}, errors.New("scheduled MQTT target does not match configuration")
		}
		run, err := endpoint.prepare(execution.MQTTBridgeRequest{ExpectedTarget: target, ExpectedPasswordRef: options.PasswordRef, Mode: options.Mode, Topic: options.Topic, Payload: options.Payload, Encoding: options.Encoding, QoS: options.QoS, Retain: options.Retain, ReceiveLimit: options.ReceiveLimit})
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
