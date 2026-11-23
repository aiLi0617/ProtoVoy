package main

import (
	"errors"
	"net/http"
	"strings"

	"protovoy.dev/protovoy/internal/execution"
)

type amqpEndpoint struct {
	target      string
	broker      execution.AMQPBroker
	passwordRef string
}

func validateAMQPPasswordReference(target, username, password, reference string) error {
	if reference != "" && (!execution.ValidPasswordReference(reference) || !strings.HasPrefix(target, "amqps://") || username == "" || password == "") {
		return errors.New("hosted AMQP password references require a valid name, TLS endpoint, username and configured password")
	}
	return nil
}

func amqpHandler(origin string, endpoint *amqpEndpoint) http.HandlerFunc {
	if endpoint == nil {
		endpoint = &amqpEndpoint{}
	}
	return brokerHandler(origin, "AMQP", endpoint.target, func(input execution.AMQPBridgeRequest) string {
		return input.ExpectedTarget
	}, func(input execution.AMQPBridgeRequest) (brokerRunner, error) {
		return endpoint.prepare(input)
	}, func(input execution.AMQPBridgeRequest) int { return input.TimeoutMS })
}

func (endpoint *amqpEndpoint) prepare(input execution.AMQPBridgeRequest) (brokerRunner, error) {
	if input.ExpectedPasswordRef != "" && (!execution.ValidPasswordReference(input.ExpectedPasswordRef) || input.ExpectedPasswordRef != endpoint.passwordRef) {
		return nil, errors.New("AMQP password reference does not match the configured endpoint")
	}
	if input.ExchangeType == "" {
		input.ExchangeType = "direct"
	}
	if input.ContentType == "" {
		input.ContentType = "application/octet-stream"
	}
	if input.ReceiveLimit == 0 {
		input.ReceiveLimit = 1
	}
	request, err := endpoint.broker.Prepare(execution.AMQPOptions{
		Mode: input.Mode, Exchange: input.Exchange, ExchangeType: input.ExchangeType,
		RoutingKey: input.RoutingKey, Queue: input.Queue, Payload: input.Payload, Encoding: input.Encoding,
		ContentType: input.ContentType, Durable: input.Durable, AutoAck: input.AutoAck, ReceiveLimit: input.ReceiveLimit,
		Declare: input.Declare == nil || *input.Declare,
	})
	return request.Execute, err
}
