package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"protovoy.dev/protovoy/internal/execution"
)

func grpcHandler(origin, target string, stopping <-chan struct{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r, origin) || r.Header.Get("X-ProtoVoy-Preview") != "1" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if target == "" {
			http.Error(w, "gRPC endpoint is not configured", http.StatusServiceUnavailable)
			return
		}
		var request execution.GRPCRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
			http.Error(w, "invalid gRPC request", http.StatusBadRequest)
			return
		}
		if request.Target != "" && request.Target != target {
			http.Error(w, "gRPC target mismatch", http.StatusConflict)
			return
		}
		if !execution.ValidRequestTimeout(request.TimeoutMS) {
			http.Error(w, "invalid gRPC timeout", http.StatusBadRequest)
			return
		}
		controller := http.NewResponseController(w)
		if request.TimeoutMS > execution.DefaultRequestTimeoutMS {
			deadline := time.Now().Add(execution.RequestTimeout(request.TimeoutMS) + 5*time.Second)
			if err := controller.SetWriteDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
				http.Error(w, "could not prepare gRPC execution", http.StatusInternalServerError)
				return
			}
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Cache-Control", "no-store")
		encoder := json.NewEncoder(w)
		send := func(event execution.GRPCBridgeEvent) error {
			if err := encoder.Encode(event); err != nil {
				return err
			}
			return controller.Flush()
		}
		request.Target = target
		ctx, cancel := streamShutdownContext(r.Context(), stopping)
		defer cancel()
		err := (execution.GRPCClient{}).Execute(ctx, request, func(status int, _ http.Header) error {
			return send(execution.GRPCBridgeEvent{Type: "status", Status: status})
		}, func(data []byte, jsonPreview string) error {
			return send(execution.GRPCBridgeEvent{Type: "message", Size: len(data), Preview: data[:min(len(data), execution.GRPCMessagePreviewLimit)], JSONPreview: jsonPreview})
		})
		if errors.Is(context.Cause(ctx), errServerStopping) {
			_ = send(execution.GRPCBridgeEvent{Type: "failed", Error: "gRPC execution canceled because the server is shutting down"})
			return
		}
		if ctx.Err() != nil {
			return
		}
		event := execution.GRPCBridgeEvent{Type: "completed"}
		if err != nil {
			event.Type, event.Error = "failed", err.Error()
			if errors.Is(err, context.DeadlineExceeded) {
				event.Code, event.Error = "timeout", "gRPC execution timed out"
			}
		}
		_ = send(event)
	}
}
