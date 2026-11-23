package main

import (
	"context"
	"errors"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

func executeCollectionMQTT(ctx context.Context, request project.Request, variables map[string]string) error {
	settings := request.MQTT
	options := execution.MQTTOptions{
		Mode: settings.Mode, Topic: settings.Topic, Encoding: settings.Encoding,
		QoS: settings.QoS, Retain: settings.Retain, ReceiveLimit: settings.ReceiveLimit,
		ClientID: settings.ClientID, Username: settings.Username, CleanSession: !settings.PersistentSession,
		PasswordRef: settings.PasswordRef,
		KeepAlive:   settings.KeepAlive, ServerName: settings.ServerName, CAPEM: settings.CAPEM,
	}
	if options.Mode == "publish" {
		options.Payload = request.Body
	}
	target, err := options.ResolveVariables(request.URL, variables, request.Variables)
	if err != nil {
		return err
	}
	prepared, err := prepareMQTT(target, options)
	if err != nil {
		return err
	}
	return executeCollectionBroker(ctx, request.BrokerTimeoutMS, prepared.Execute, func(any) error { return nil })
}

func executeCollectionAMQP(ctx context.Context, request project.Request, variables map[string]string) error {
	settings := request.AMQP
	options := execution.AMQPOptions{
		Mode: settings.Mode, Exchange: settings.Exchange, ExchangeType: settings.ExchangeType,
		RoutingKey: settings.RoutingKey, Queue: settings.Queue, Encoding: settings.Encoding, ContentType: settings.ContentType,
		Declare: settings.Declare, Durable: settings.Durable, AutoAck: settings.AutoAck, ReceiveLimit: settings.ReceiveLimit,
		Username: settings.Username, ServerName: settings.ServerName, CAPEM: settings.CAPEM,
		PasswordRef: settings.PasswordRef,
	}
	if options.Mode == "publish" {
		options.Payload = request.Body
	}
	target, err := options.ResolveVariables(request.URL, variables, request.Variables)
	if err != nil {
		return err
	}
	prepared, err := prepareAMQP(target, options)
	if err != nil {
		return err
	}
	return executeCollectionBroker(ctx, request.BrokerTimeoutMS, prepared.Execute, func(result any) error {
		if options.Mode != "publish" {
			return nil
		}
		confirmation, ok := result.(map[string]any)
		if !ok || confirmation["published"] != true {
			return errors.New("AMQP publication was not confirmed or was returned")
		}
		return nil
	})
}

func executeCollectionKafka(ctx context.Context, request project.Request, variables map[string]string) error {
	settings := request.Kafka
	operation, err := settings.Operation(request.Body)
	if err != nil {
		return err
	}
	options := execution.KafkaOptions{
		KafkaOperation: operation, KafkaTLSReferences: settings.KafkaTLSReferences, PasswordRef: settings.PasswordRef,
		SecurityProtocol: settings.SecurityProtocol, SASLMechanism: settings.SASLMechanism, Username: settings.Username,
	}
	target, err := options.ResolveVariables(request.URL, variables, request.Variables)
	if err != nil {
		return err
	}
	prepared, err := prepareCLIKafka(target, options)
	if err != nil {
		return err
	}
	return executeCollectionBroker(ctx, request.BrokerTimeoutMS, prepared.Execute, func(any) error { return nil })
}

func executeCollectionBroker(ctx context.Context, timeoutMS int, execute func(context.Context, func() error, func(any) error) error, emit func(any) error) error {
	ctx, cancel := context.WithTimeout(ctx, execution.RequestTimeout(timeoutMS))
	defer cancel()
	err := execute(ctx, func() error { return nil }, emit)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
