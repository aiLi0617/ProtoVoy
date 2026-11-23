package main

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/secrets"
)

func runAMQP(ctx context.Context, target string, options execution.AMQPOptions, caFile string, timeout time.Duration, output io.Writer) error {
	if options.Mode == "consume" && options.Payload != "" {
		return errors.New("-data applies only to AMQP publish")
	}
	if options.Mode == "publish" && options.Queue != "" {
		return errors.New("use -amqp-routing-key for publish or -amqp-queue with -amqp-consume")
	}
	var err error
	options.CAPEM, err = readBrokerCA(caFile)
	if err != nil {
		return err
	}
	request, err := prepareAMQP(target, options)
	if err != nil {
		return err
	}
	return runBroker(ctx, timeout, output, request.Execute)
}

func prepareAMQP(target string, options execution.AMQPOptions) (execution.AMQPRequest, error) {
	resolve := func(string) (string, error) { return os.Getenv(execution.AMQPPasswordEnv), nil }
	if os.Getenv("PROTOVOY_AMQP_PASSWORD_KEYCHAIN") == "1" {
		resolve = secrets.Resolve
	}
	return execution.PrepareAMQPWithPasswordResolver(target, options, resolve)
}
