package main

import (
	"errors"
	"os"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/secrets"
)

func (envelope requestEnvelope) prepareAMQP(useKeychain ...bool) (execution.AMQPRequest, error) {
	var input struct {
		Type  string `json:"type"`
		Value *struct {
			Mode, Username, Password, PasswordRef, ServerName, CAPEM string
			Exchange, ExchangeType, RoutingKey, Queue                string
			Payload, Encoding, ContentType                           string
			Declare                                                  *bool
			Durable, AutoAck                                         bool
			Literal                                                  bool
			ReceiveLimit                                             *int
		} `json:"value"`
	}
	if !decodeExecutionPayload(envelope.Payload, &input) || input.Type != "raw" || input.Value == nil {
		return execution.AMQPRequest{}, errors.New("invalid AMQP execution payload")
	}
	value := input.Value
	options := execution.AMQPOptions{
		Mode: value.Mode, Username: value.Username, Password: value.Password, ServerName: value.ServerName, CAPEM: value.CAPEM,
		PasswordRef: value.PasswordRef,
		Exchange:    value.Exchange, ExchangeType: value.ExchangeType, RoutingKey: value.RoutingKey, Queue: value.Queue,
		Payload: value.Payload, Encoding: value.Encoding, ContentType: value.ContentType,
		Declare: true, Durable: value.Durable, AutoAck: value.AutoAck, ReceiveLimit: 1,
	}
	if options.Mode == "" {
		options.Mode = "publish"
	}
	if options.ExchangeType == "" {
		options.ExchangeType = "direct"
	}
	if options.ContentType == "" {
		options.ContentType = "application/octet-stream"
	}
	if value.Declare != nil {
		options.Declare = *value.Declare
	}
	if value.ReceiveLimit != nil {
		options.ReceiveLimit = *value.ReceiveLimit
	}
	target := envelope.Target
	if !value.Literal {
		var err error
		target, err = options.ResolveVariables(target, envelope.Variables)
		if err != nil {
			return execution.AMQPRequest{}, err
		}
	}
	resolve := func(string) (string, error) { return os.Getenv(execution.AMQPPasswordEnv), nil }
	if len(useKeychain) > 0 && useKeychain[0] {
		resolve = secrets.Resolve
	}
	return execution.PrepareAMQPWithPasswordResolver(target, options, resolve)
}
