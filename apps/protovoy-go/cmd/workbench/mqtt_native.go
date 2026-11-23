//go:build !js

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/secrets"
)

func runDirectMQTT(ctx context.Context, target string, options execution.MQTTOptions, connected func(), message func(json.RawMessage)) error {
	request, err := prepareDirectMQTT(target, options)
	if err != nil {
		return err
	}
	return runNativeBroker(ctx, options.TimeoutMS, request.Execute, connected, message)
}

func prepareDirectMQTT(target string, options execution.MQTTOptions) (execution.MQTTRequest, error) {
	resolve := func(string) (string, error) { return os.Getenv(execution.MQTTPasswordEnv), nil }
	if os.Getenv("PROTOVOY_MQTT_PASSWORD_KEYCHAIN") == "1" {
		resolve = secrets.Resolve
	}
	return execution.PrepareMQTTWithPasswordResolver(target, options, resolve)
}

func runDirectAMQP(ctx context.Context, target string, options execution.AMQPOptions, connected func(), message func(json.RawMessage)) error {
	request, err := prepareDirectAMQP(target, options)
	if err != nil {
		return err
	}
	return runNativeBroker(ctx, options.TimeoutMS, request.Execute, connected, message)
}

func prepareDirectAMQP(target string, options execution.AMQPOptions) (execution.AMQPRequest, error) {
	resolve := func(string) (string, error) { return os.Getenv(execution.AMQPPasswordEnv), nil }
	if os.Getenv("PROTOVOY_AMQP_PASSWORD_KEYCHAIN") == "1" {
		resolve = secrets.Resolve
	}
	return execution.PrepareAMQPWithPasswordResolver(target, options, resolve)
}

func runDirectKafka(ctx context.Context, target string, options execution.KafkaOptions, connected func(), message func(json.RawMessage)) error {
	request, err := prepareDirectKafka(target, options)
	if err != nil {
		return err
	}
	return runNativeBroker(ctx, options.TimeoutMS, request.Execute, connected, message)
}

func prepareDirectKafka(target string, options execution.KafkaOptions) (execution.KafkaRequest, error) {
	if os.Getenv("PROTOVOY_KAFKA_PASSWORD_KEYCHAIN") == "1" {
		return execution.PrepareKafkaWithPasswordResolver(target, options, secrets.Resolve)
	}
	return execution.PrepareKafka(target, options)
}

func runNativeBroker(ctx context.Context, timeoutMS int, execute func(context.Context, func() error, func(any) error) error, connected func(), message func(json.RawMessage)) error {
	ctx, cancel := context.WithTimeout(ctx, execution.RequestTimeout(timeoutMS))
	defer cancel()
	total := 0
	err := execute(ctx, func() error {
		connected()
		return nil
	}, func(value any) error {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		total += len(data) + 1
		if total > execution.BrokerResultLimit {
			return errors.New("broker result exceeds 16 MiB")
		}
		message(data)
		return nil
	})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
