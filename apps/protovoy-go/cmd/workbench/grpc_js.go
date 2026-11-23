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

func runDirectGRPC(ctx context.Context, request execution.GRPCRequest, onStatus func(int), onMessage func(execution.GRPCMessage)) error {
	body, err := json.Marshal(request)
	if err != nil || len(body) > 2<<20 {
		return errors.New("could not encode gRPC request within 2 MiB")
	}
	origin := js.Global().Get("location").Get("origin").String()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, origin+execution.GRPCBridgePath, bytes.NewReader(body))
	if err != nil {
		return errors.New("could not prepare gRPC bridge request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ProtoVoy-Preview", "1")
	response, err := fetchStreamingResponse(req, "same-origin", "same-origin")
	if err != nil {
		return errors.New("could not reach the gRPC execution service")
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusServiceUnavailable:
		return errors.New("server gRPC endpoint is not configured")
	case http.StatusConflict:
		return errors.New("gRPC URL differs from the server-configured endpoint")
	case http.StatusOK:
	default:
		return errors.New("server rejected the gRPC request")
	}
	media, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if media != "application/x-ndjson" {
		return errors.New("invalid gRPC bridge response")
	}
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		var event execution.GRPCBridgeEvent
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			return errors.New("invalid gRPC bridge event")
		}
		switch event.Type {
		case "status":
			onStatus(event.Status)
		case "message":
			if event.Size < len(event.Preview) || event.Size > execution.GRPCMessageLimit ||
				len(event.Preview) > execution.GRPCMessagePreviewLimit || len(event.JSONPreview) > execution.GRPCMessagePreviewLimit {
				return errors.New("invalid gRPC message preview")
			}
			onMessage(execution.GRPCMessage{Size: event.Size, Preview: event.Preview, JSONPreview: event.JSONPreview})
		case "failed":
			if event.Code == "timeout" {
				return context.DeadlineExceeded
			}
			return errors.New(event.Error)
		case "completed":
			return nil
		default:
			return errors.New("invalid gRPC bridge event type")
		}
	}
	return errors.New("gRPC bridge stream ended before completion")
}
