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

func runMQTT(ctx context.Context, target string, options execution.MQTTOptions, caFile string, timeout time.Duration, output io.Writer) error {
	if options.Mode == "subscribe" && (options.Payload != "" || options.Retain) {
		return errors.New("-data and -mqtt-retain apply only to MQTT publish")
	}
	var err error
	options.CAPEM, err = readBrokerCA(caFile)
	if err != nil {
		return err
	}
	request, err := prepareMQTT(target, options)
	if err != nil {
		return err
	}
	return runBroker(ctx, timeout, output, request.Execute)
}

func prepareMQTT(target string, options execution.MQTTOptions) (execution.MQTTRequest, error) {
	resolve := func(string) (string, error) { return os.Getenv(execution.MQTTPasswordEnv), nil }
	if os.Getenv("PROTOVOY_MQTT_PASSWORD_KEYCHAIN") == "1" {
		resolve = secrets.Resolve
	}
	return execution.PrepareMQTTWithPasswordResolver(target, options, resolve)
}
