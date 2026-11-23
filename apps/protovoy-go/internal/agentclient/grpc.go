package agentclient

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"protovoy.dev/protovoy/internal/execution"
)

func (client *Client) RunGRPC(ctx context.Context, request execution.GRPCRequest, onStatus func(int), onMessage func(execution.GRPCMessage)) error {
	if !execution.ValidRequestTimeout(request.TimeoutMS) {
		return errors.New("invalid gRPC timeout")
	}
	timeout := execution.RequestTimeout(request.TimeoutMS)
	ctx, cancel := context.WithTimeout(ctx, timeout+5*time.Second)
	defer cancel()
	stream, err := client.openExecutionWithVariables(ctx, request.Target, "grpc", map[string]any{
		"type": "grpc", "service": request.Service, "method": request.Method, "mode": request.Mode,
		"messageBase64": request.MessageBase64, "metadata": request.Metadata, "literal": true,
		"messageJson": request.MessageJSON, "descriptorSetBase64": request.DescriptorSetBase64,
	}, nil, int(timeout/time.Millisecond))
	if err != nil {
		return err
	}
	defer stream.Close()
	decoder := execution.NewSSEDecoder(stream.body, "", 0)
	var message execution.GRPCMessage
	var count int
	for {
		event, err := nextAgentEvent(decoder)
		if err != nil {
			return err
		}
		switch event.Type {
		case "response_meta":
			onStatus(event.Status)
		case "response_chunk":
			if event.Done && event.MessageIndex == 0 {
				continue
			}
			chunk, err := base64.StdEncoding.DecodeString(event.DataBase64)
			if err != nil || len(chunk) != event.Size || event.MessageIndex != count+1 ||
				(len(chunk) == 0 && !event.MessageEnd) || message.Size > execution.GRPCMessageLimit-len(chunk) {
				return errors.New("Local Agent returned invalid gRPC message data")
			}
			message.Size += len(chunk)
			remaining := execution.GRPCMessagePreviewLimit - len(message.Preview)
			message.Preview = append(message.Preview, chunk[:min(remaining, len(chunk))]...)
			if event.MessageEnd {
				if len(event.JSONPreview) > execution.GRPCMessagePreviewLimit {
					return errors.New("Local Agent returned an oversized gRPC JSON preview")
				}
				message.JSONPreview = event.JSONPreview
				onMessage(message)
				count++
				message = execution.GRPCMessage{}
			}
		case "completed":
			stream.finished = true
			if message.Size != 0 {
				return errors.New("Local Agent completed during a partial gRPC message")
			}
			return nil
		case "failed":
			stream.finished = true
			if event.Code == "timeout" {
				return context.DeadlineExceeded
			}
			if event.GRPCStatus != nil && *event.GRPCStatus > 0 && *event.GRPCStatus <= 16 {
				return fmt.Errorf("gRPC status %d", *event.GRPCStatus)
			}
			if event.Code == "grpc_reflection" {
				return execution.ErrGRPCReflection
			}
			return errors.New("Local Agent gRPC execution failed")
		case "cancelled":
			stream.finished = true
			return context.Canceled
		}
	}
}
