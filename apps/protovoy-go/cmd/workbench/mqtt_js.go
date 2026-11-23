package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"syscall/js"

	"protovoy.dev/protovoy/internal/execution"
)

func runDirectMQTT(ctx context.Context, target string, options execution.MQTTOptions, connected func(), message func(json.RawMessage)) error {
	input := execution.MQTTBridgeRequest{
		ExpectedPasswordRef: options.PasswordRef,
		TimeoutMS:           options.TimeoutMS,
		ExpectedTarget:      target, Mode: options.Mode, Topic: options.Topic, Payload: options.Payload,
		Encoding: options.Encoding, QoS: options.QoS, Retain: options.Retain, ReceiveLimit: options.ReceiveLimit,
	}
	return runBrokerBridge(ctx, execution.MQTTBridgePath, "MQTT", input, connected, message)
}

func runDirectAMQP(ctx context.Context, target string, options execution.AMQPOptions, connected func(), message func(json.RawMessage)) error {
	input := execution.AMQPBridgeRequest{
		ExpectedPasswordRef: options.PasswordRef,
		TimeoutMS:           options.TimeoutMS,
		ExpectedTarget:      target, Mode: options.Mode, Exchange: options.Exchange, ExchangeType: options.ExchangeType,
		RoutingKey: options.RoutingKey, Queue: options.Queue, Payload: options.Payload, Encoding: options.Encoding,
		ContentType: options.ContentType, Declare: &options.Declare, Durable: options.Durable,
		AutoAck: options.AutoAck, ReceiveLimit: options.ReceiveLimit,
	}
	return runBrokerBridge(ctx, execution.AMQPBridgePath, "AMQP", input, connected, message)
}

func runDirectKafka(ctx context.Context, target string, options execution.KafkaOptions, connected func(), message func(json.RawMessage)) error {
	return runBrokerBridge(ctx, execution.KafkaBridgePath, "Kafka", execution.KafkaBridgeRequest{
		TimeoutMS:      options.TimeoutMS,
		ExpectedTarget: target, ExpectedPasswordRef: options.PasswordRef, KafkaOperation: options.KafkaOperation,
		ExpectedTLSReferences: options.KafkaTLSReferences,
	}, connected, message)
}

func runBrokerBridge(ctx context.Context, path, protocol string, input any, connected func(), message func(json.RawMessage)) error {
	body, err := json.Marshal(input)
	if err != nil || len(body) > 2<<20 {
		return errors.New("could not encode " + protocol + " request within 2 MiB")
	}
	origin := js.Global().Get("location").Get("origin").String()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, origin+path, bytes.NewReader(body))
	if err != nil {
		return errors.New("could not prepare broker bridge request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ProtoVoy-Preview", "1")
	response, err := fetchStreamingResponse(req, "same-origin", "same-origin")
	if err != nil {
		return errors.New("could not reach the " + protocol + " execution service")
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusServiceUnavailable:
		return errors.New(protocol + " execution service is unavailable")
	case http.StatusConflict:
		return errors.New(protocol + " target differs from the server-configured endpoint")
	case http.StatusOK:
	default:
		return errors.New("server rejected the " + protocol + " request")
	}
	media, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if media != "application/x-ndjson" {
		return errors.New("invalid broker bridge response")
	}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), execution.BrokerResultLimit+1024)
	total := 0
	ready := false
	for scanner.Scan() {
		var event execution.BrokerBridgeEvent
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			return errors.New("invalid broker bridge event")
		}
		switch event.Type {
		case "connected":
			if ready {
				return errors.New("duplicate broker connection event")
			}
			ready = true
			connected()
		case "message":
			total += len(event.Message) + 1
			if !ready || len(event.Message) == 0 || total > execution.BrokerResultLimit {
				return errors.New("invalid broker message data")
			}
			message(event.Message)
		case "failed":
			return errors.New(event.Error)
		case "completed":
			if ready {
				return nil
			}
			return errors.New(protocol + " execution completed before connecting")
		default:
			return errors.New("invalid broker bridge event type")
		}
	}
	return errors.New(protocol + " bridge stream ended before completion")
}
