package main

import (
	"crypto/rand"
	"errors"
	"net/http"
	"strings"

	"protovoy.dev/protovoy/internal/execution"
)

type mqttEndpoint struct {
	target      string
	broker      execution.MQTTBroker
	username    string
	password    string
	passwordRef string
}

func validateMQTTPasswordReference(endpoint *mqttEndpoint) error {
	if endpoint.passwordRef != "" && (!execution.ValidPasswordReference(endpoint.passwordRef) || !strings.HasPrefix(endpoint.target, "mqtts://") || endpoint.username == "" || endpoint.password == "") {
		return errors.New("hosted MQTT password references require a valid name, TLS endpoint, username and configured password")
	}
	return nil
}

func mqttHandler(origin string, endpoint *mqttEndpoint) http.HandlerFunc {
	if endpoint == nil {
		endpoint = &mqttEndpoint{}
	}
	return brokerHandler(origin, "MQTT", endpoint.target, func(input execution.MQTTBridgeRequest) string {
		return input.ExpectedTarget
	}, func(input execution.MQTTBridgeRequest) (brokerRunner, error) {
		return endpoint.prepare(input)
	}, func(input execution.MQTTBridgeRequest) int { return input.TimeoutMS })
}

func (endpoint *mqttEndpoint) prepare(input execution.MQTTBridgeRequest) (brokerRunner, error) {
	if input.ExpectedPasswordRef != "" && (!execution.ValidPasswordReference(input.ExpectedPasswordRef) || input.ExpectedPasswordRef != endpoint.passwordRef) {
		return nil, errors.New("MQTT password reference does not match the configured endpoint")
	}
	if input.ReceiveLimit == 0 {
		input.ReceiveLimit = 1
	}
	request, err := endpoint.broker.Prepare(execution.MQTTOptions{
		Mode: input.Mode, Topic: input.Topic, Payload: input.Payload, Encoding: input.Encoding,
		QoS: input.QoS, Retain: input.Retain, ReceiveLimit: input.ReceiveLimit,
		ClientID: "pv" + rand.Text()[:21], CleanSession: true, KeepAlive: 15,
		Username: endpoint.username, Password: endpoint.password,
	})
	return request.Execute, err
}
