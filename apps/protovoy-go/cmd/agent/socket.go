package main

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"strings"
	"time"

	"protovoy.dev/protovoy/internal/execution"
)

func (envelope requestEnvelope) prepareSocket(timeout time.Duration) (execution.SocketRequest, error) {
	var input struct {
		execution.SocketPayload
		Type      string `json:"type"`
		SendCount *int   `json:"sendCount"`
		CARef     string `json:"caCertRef"`
	}
	if !decodeExecutionPayload(envelope.Payload, &input) || input.Type != envelope.Protocol ||
		input.Network != "" || input.ExpectedTarget != "" || input.TimeoutMS != 0 {
		return execution.SocketRequest{}, errors.New("invalid socket execution payload")
	}
	if input.CARef != "" {
		return execution.SocketRequest{}, errors.New("stored CA references are not available; provide CA certificates as PEM")
	}
	if !input.SocketPayload.ValidSendOptions() ||
		(input.SendCount != nil && (*input.SendCount < 1 || *input.SendCount > 10_000)) {
		return execution.SocketRequest{}, errors.New("use 1 to 10000 sends and a valid interval")
	}
	payload := input.SocketPayload
	payload.Network, payload.ExpectedTarget = envelope.Protocol, envelope.Target
	payload, err := payload.ResolveVariables(envelope.Variables)
	if err != nil {
		return execution.SocketRequest{}, err
	}
	target := strings.TrimPrefix(payload.ExpectedTarget, payload.Network+"://")
	host, port, err := net.SplitHostPort(target)
	if err != nil || host == "" || port == "" || strings.ContainsAny(target, "@/\r\n\x00") {
		return execution.SocketRequest{}, errors.New("enter a host:port socket target")
	}
	request := payload.Request(target)
	request.Timeout = timeout
	if input.SendCount != nil {
		request.SendCount = *input.SendCount
	}
	return request, nil
}

func (job *agentJob) executeSocket(ctx context.Context, id, requestID string, request execution.SocketRequest) agentEvent {
	started := time.Now()
	if !job.emit(ctx, agentEvent{"type": "state_changed", "state": "running", "phase": "connect"}) {
		return terminalFailure(ctx, "connection")
	}
	data, err := execution.ExchangeSocket(ctx, request)
	const contentType = "application/octet-stream"
	if !job.emit(ctx, agentEvent{
		"type": "response_meta", "status": nil, "contentType": contentType, "headers": [][2]string{},
	}) {
		return terminalFailure(ctx, "transfer")
	}
	if !job.emitBinary(ctx, contentType, data) {
		return terminalFailure(ctx, "transfer")
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return agentEvent{"type": "failed", "code": "timeout", "message": "socket request timed out"}
		}
		return terminalFailure(ctx, "transfer")
	}
	if !job.emit(ctx, agentEvent{
		"type": "response_chunk", "content_type": contentType, "size": 0, "done": true,
		"preview": base64.StdEncoding.EncodeToString(data[:min(len(data), execution.PreviewLimit)]),
	}) {
		return terminalFailure(ctx, "transfer")
	}
	return completedEvent(id, requestID, request.Network, 0, int64(len(data)), started)
}
