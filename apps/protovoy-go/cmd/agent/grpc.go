package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"time"

	"protovoy.dev/protovoy/internal/execution"
)

func (envelope requestEnvelope) prepareGRPC(timeout time.Duration) (execution.GRPCRequest, error) {
	var input struct {
		Type                string      `json:"type"`
		Service             string      `json:"service"`
		Method              string      `json:"method"`
		Mode                string      `json:"mode"`
		MessageBase64       string      `json:"messageBase64"`
		MessageJSON         string      `json:"messageJson"`
		DescriptorSetBase64 string      `json:"descriptorSetBase64"`
		Metadata            [][2]string `json:"metadata"`
		Literal             bool        `json:"literal"`
	}
	if !decodeExecutionPayload(envelope.Payload, &input) || input.Type != "grpc" {
		return execution.GRPCRequest{}, errors.New("invalid gRPC execution payload")
	}
	request := execution.GRPCRequest{
		Target: envelope.Target, Service: input.Service, Method: input.Method, Mode: input.Mode,
		MessageBase64: input.MessageBase64, Metadata: input.Metadata, TimeoutMS: int(timeout / time.Millisecond),
		MessageJSON: input.MessageJSON, DescriptorSetBase64: input.DescriptorSetBase64,
	}
	if !input.Literal {
		return request.ResolveVariables(envelope.Variables)
	}
	return request, nil
}

func (job *agentJob) executeGRPC(ctx context.Context, id, requestID string, request execution.GRPCRequest) agentEvent {
	started := time.Now()
	if !job.emit(ctx, agentEvent{"type": "state_changed", "state": "running", "phase": "connect"}) {
		return terminalFailure(ctx, "connection")
	}
	var status, index int
	var total int64
	var preview string
	const contentType = "application/protobuf"
	err := (execution.GRPCClient{}).Execute(ctx, request, func(code int, headers http.Header) error {
		status = code
		if !job.emitResponseMeta(ctx, &execution.HTTPResponse{StatusCode: code, Headers: headers}) {
			return ctx.Err()
		}
		return nil
	}, func(message []byte, jsonPreview string) error {
		index++
		total += int64(len(message))
		preview = fmt.Sprintf("gRPC message #%d: %d bytes", index, len(message))
		if jsonPreview != "" {
			preview = jsonPreview
		} else if len(message) <= execution.PreviewLimit {
			preview += ", base64=" + base64.StdEncoding.EncodeToString(message)
		}
		for offset := 0; ; offset += execution.PreviewLimit {
			end := min(offset+execution.PreviewLimit, len(message))
			event := agentEvent{
				"type": "response_chunk", "content_type": contentType, "size": end - offset,
				"data_base64":  base64.StdEncoding.EncodeToString(message[offset:end]),
				"messageIndex": index, "messageEnd": end == len(message), "done": false,
			}
			if end == len(message) {
				event["preview"] = preview
				if jsonPreview != "" {
					event["jsonPreview"] = jsonPreview
				}
			}
			if !job.emit(ctx, event) {
				return ctx.Err()
			}
			if end == len(message) {
				return nil
			}
		}
	})
	if err != nil {
		var grpcStatus execution.GRPCStatusError
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return agentEvent{"type": "failed", "code": "timeout", "message": "gRPC execution timed out"}
		case ctx.Err() != nil || errors.Is(err, context.Canceled):
			return terminalFailure(ctx, "transfer")
		case errors.As(err, &grpcStatus):
			return agentEvent{"type": "failed", "code": "grpc_status", "message": grpcStatus.Error(), "grpcStatus": grpcStatus.Code}
		case errors.Is(err, execution.ErrGRPCReflection):
			return agentEvent{"type": "failed", "code": "grpc_reflection", "message": err.Error()}
		default:
			return agentEvent{"type": "failed", "code": "grpc_protocol", "message": err.Error()}
		}
	}
	if !job.emit(ctx, agentEvent{"type": "response_chunk", "content_type": contentType, "size": 0, "preview": preview, "done": true}) {
		return terminalFailure(ctx, "transfer")
	}
	return completedEvent(id, requestID, "grpc", status, total, started)
}
