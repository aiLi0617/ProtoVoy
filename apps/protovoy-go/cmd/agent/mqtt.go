package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/secrets"
)

func (envelope requestEnvelope) prepareMQTT(useKeychain ...bool) (execution.MQTTRequest, error) {
	var input struct {
		Type  string `json:"type"`
		Value *struct {
			Mode         string `json:"mode"`
			ClientID     string `json:"clientId"`
			Username     string `json:"username"`
			Password     string `json:"password"`
			PasswordRef  string `json:"passwordRef"`
			CleanSession *bool  `json:"cleanSession"`
			KeepAlive    *int   `json:"keepAliveSeconds"`
			Topic        string `json:"topic"`
			Payload      string `json:"payload"`
			Encoding     string `json:"encoding"`
			QoS          int    `json:"qos"`
			Retain       bool   `json:"retain"`
			ReceiveLimit *int   `json:"receiveLimit"`
			CAPEM        string `json:"caPem"`
			CAPEMRef     string `json:"caPemRef"`
			ServerName   string `json:"serverName"`
			Literal      bool   `json:"literal"`
		} `json:"value"`
	}
	if !decodeExecutionPayload(envelope.Payload, &input) || input.Type != "raw" || input.Value == nil {
		return execution.MQTTRequest{}, errors.New("invalid MQTT execution payload")
	}
	value := input.Value
	if value.CAPEMRef != "" {
		return execution.MQTTRequest{}, errors.New("MQTT CA references are not available")
	}
	options := execution.MQTTOptions{
		Mode: value.Mode, ClientID: value.ClientID, Username: value.Username, Password: value.Password,
		PasswordRef:  value.PasswordRef,
		CleanSession: true, KeepAlive: 30, ReceiveLimit: 1, QoS: value.QoS, Retain: value.Retain,
		Topic: value.Topic, Payload: value.Payload, Encoding: value.Encoding, CAPEM: value.CAPEM, ServerName: value.ServerName,
	}
	if options.Mode == "" {
		options.Mode = "publish"
	}
	if value.CleanSession != nil {
		options.CleanSession = *value.CleanSession
	}
	if value.KeepAlive != nil {
		options.KeepAlive = *value.KeepAlive
	}
	if value.ReceiveLimit != nil {
		options.ReceiveLimit = *value.ReceiveLimit
	}
	target := envelope.Target
	if !value.Literal {
		var err error
		target, err = options.ResolveVariables(target, envelope.Variables)
		if err != nil {
			return execution.MQTTRequest{}, err
		}
	}
	resolve := func(string) (string, error) { return os.Getenv(execution.MQTTPasswordEnv), nil }
	if len(useKeychain) > 0 && useKeychain[0] {
		resolve = secrets.Resolve
	}
	return execution.PrepareMQTTWithPasswordResolver(target, options, resolve)
}

func (job *agentJob) executeMQTT(ctx context.Context, id, requestID string, request execution.MQTTRequest) agentEvent {
	return job.executeJSONMessages(ctx, id, requestID, "mqtt", "MQTT connected", request.Execute)
}

func (job *agentJob) executeJSONMessages(ctx context.Context, id, requestID, protocol, status string, run func(context.Context, func() error, func(any) error) error) agentEvent {
	started := time.Now()
	if !job.emit(ctx, agentEvent{"type": "state_changed", "state": "running", "phase": "connect"}) {
		return terminalFailure(ctx, "connection")
	}
	const contentType = "application/json"
	var total int64
	err := run(ctx, func() error {
		if !job.emit(ctx, agentEvent{"type": "response_meta", "status": nil, "statusText": status, "contentType": contentType, "headers": [][2]string{}}) ||
			!job.emit(ctx, agentEvent{"type": "state_changed", "state": "running", "phase": "transfer"}) {
			return ctx.Err()
		}
		return nil
	}, func(message any) error {
		data, err := json.Marshal(message)
		if err != nil || total+int64(len(data))+1 > 16<<20 {
			return errors.New("JSON result exceeds 16 MiB")
		}
		data = append(data, '\n')
		if !job.emitBinary(ctx, contentType, data) {
			return ctx.Err()
		}
		total += int64(len(data))
		return nil
	})
	if ctx.Err() != nil {
		return terminalFailure(ctx, "transfer")
	}
	if err != nil {
		return agentEvent{"type": "failed", "code": protocol + "_protocol", "message": err.Error()}
	}
	if !job.emit(ctx, agentEvent{"type": "response_chunk", "content_type": contentType, "size": 0, "done": true}) {
		return terminalFailure(ctx, "transfer")
	}
	return completedEvent(id, requestID, protocol, 0, total, started)
}
