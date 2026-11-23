package agentclient

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"protovoy.dev/protovoy/internal/execution"
)

func (client *Client) RunMQTT(ctx context.Context, target string, options execution.MQTTOptions, connected func(), message func(json.RawMessage)) error {
	return client.runBroker(ctx, target, "mqtt", map[string]any{
		"type": "raw", "value": map[string]any{
			"literal": true, "mode": options.Mode, "topic": options.Topic, "qos": options.QoS,
			"payload": options.Payload, "encoding": options.Encoding, "retain": options.Retain, "receiveLimit": options.ReceiveLimit,
			"clientId": options.ClientID, "cleanSession": options.CleanSession, "keepAliveSeconds": options.KeepAlive,
			"username": options.Username, "password": options.Password, "passwordRef": options.PasswordRef, "caPem": options.CAPEM, "serverName": options.ServerName,
		},
	}, options.TimeoutMS, connected, message)
}

func (client *Client) RunAMQP(ctx context.Context, target string, options execution.AMQPOptions, connected func(), message func(json.RawMessage)) error {
	return client.runBroker(ctx, target, "amqp", map[string]any{
		"type": "raw", "value": map[string]any{
			"literal": true, "mode": options.Mode, "exchange": options.Exchange, "exchangeType": options.ExchangeType,
			"routingKey": options.RoutingKey, "queue": options.Queue, "payload": options.Payload, "encoding": options.Encoding,
			"contentType": options.ContentType, "declare": options.Declare, "durable": options.Durable,
			"autoAck": options.AutoAck, "receiveLimit": options.ReceiveLimit, "username": options.Username,
			"password": options.Password, "passwordRef": options.PasswordRef, "caPem": options.CAPEM, "serverName": options.ServerName,
		},
	}, options.TimeoutMS, connected, message)
}

func (client *Client) RunKafka(ctx context.Context, target string, options execution.KafkaOptions, connected func(), message func(json.RawMessage)) error {
	return client.runBroker(ctx, target, "kafka", map[string]any{
		"type": "raw", "value": struct {
			execution.KafkaOptions
			Literal bool `json:"literal"`
		}{KafkaOptions: options, Literal: true},
	}, options.TimeoutMS, connected, message)
}

func (client *Client) RunSQL(ctx context.Context, input execution.SQLBridgeRequest, credentials execution.SQLCredentials, connected func(), message func(json.RawMessage)) error {
	return client.runBroker(ctx, input.ExpectedTarget, "sql", map[string]any{
		"type": "raw", "value": map[string]any{
			"literal": true, "sql": input.Statement, "parameters": input.Parameters, "rowLimit": input.RowLimit,
			"transactional": input.Transactional, "username": credentials.Username, "password": credentials.Password,
			"passwordRef": credentials.PasswordRef,
		},
	}, input.TimeoutMS, connected, message)
}

func (client *Client) runBroker(ctx context.Context, target, protocol string, payload map[string]any, timeoutMS int, connected func(), message func(json.RawMessage)) error {
	if !execution.ValidRequestTimeout(timeoutMS) {
		return errors.New("execution timeout must be at most five minutes")
	}
	timeout := execution.RequestTimeout(timeoutMS)
	ctx, cancel := context.WithTimeout(ctx, timeout+5*time.Second)
	defer cancel()
	stream, err := client.openExecutionWithVariables(ctx, target, protocol, payload, nil, int(timeout/time.Millisecond))
	if err != nil {
		return err
	}
	defer stream.Close()
	decoder := execution.NewSSEDecoder(stream.body, "", 0)
	var pending []byte
	total := 0
	for {
		event, err := nextAgentEvent(decoder)
		if err != nil {
			return err
		}
		switch event.Type {
		case "response_meta":
			connected()
		case "response_chunk":
			if event.Done {
				continue
			}
			chunk, err := base64.StdEncoding.DecodeString(event.DataBase64)
			if err != nil || len(chunk) == 0 || len(chunk) != event.Size || total+len(chunk) > execution.BrokerResultLimit {
				return errors.New("invalid broker message data from Local Agent")
			}
			total += len(chunk)
			pending = append(pending, chunk...)
			if pending[len(pending)-1] == '\n' {
				if !json.Valid(pending[:len(pending)-1]) {
					return errors.New("invalid broker message from Local Agent")
				}
				message(pending[:len(pending)-1])
				pending = nil
			}
		case "completed":
			stream.finished = true
			if len(pending) != 0 {
				return errors.New("Local Agent completed a partial broker message")
			}
			return nil
		case "failed":
			stream.finished = true
			if event.Code == "timeout" {
				return context.DeadlineExceeded
			}
			return errors.New("Local Agent broker execution failed")
		case "cancelled":
			stream.finished = true
			return context.Canceled
		}
	}
}
