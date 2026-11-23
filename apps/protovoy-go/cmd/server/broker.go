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

type brokerRunner func(context.Context, func() error, func(any) error) error

func brokerHandler[T any](origin, protocol, target string, expectedTarget func(T) string, prepare func(T) (brokerRunner, error), timeout func(T) int) http.HandlerFunc {
	slots := make(chan struct{}, 16)
	return func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r, origin) || r.Header.Get("X-ProtoVoy-Preview") != "1" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if target == "" {
			http.Error(w, protocol+" endpoint is not configured", http.StatusServiceUnavailable)
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			http.Error(w, "too many "+protocol+" executions", http.StatusServiceUnavailable)
			return
		}
		var input T
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
			http.Error(w, "invalid "+protocol+" request", http.StatusBadRequest)
			return
		}
		if expected := expectedTarget(input); expected != "" && expected != target {
			http.Error(w, protocol+" target mismatch", http.StatusConflict)
			return
		}
		timeoutMS := 0
		if timeout != nil {
			timeoutMS = timeout(input)
		}
		if !execution.ValidRequestTimeout(timeoutMS) {
			http.Error(w, "invalid execution timeout", http.StatusBadRequest)
			return
		}
		run, err := prepare(input)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		streamBroker(w, r, protocol, run, execution.RequestTimeout(timeoutMS))
	}
}

func streamBroker(w http.ResponseWriter, r *http.Request, protocol string, run brokerRunner, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	controller := http.NewResponseController(w)
	if timeout > execution.RequestTimeout(0) {
		deadline, _ := ctx.Deadline()
		if err := controller.SetWriteDeadline(deadline.Add(5 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			http.Error(w, "could not prepare execution", http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	encoder := json.NewEncoder(w)
	send := func(event execution.BrokerBridgeEvent) error {
		if err := encoder.Encode(event); err != nil {
			return err
		}
		return controller.Flush()
	}
	total := 0
	err := run(ctx, func() error {
		return send(execution.BrokerBridgeEvent{Type: "connected"})
	}, func(message any) error {
		data, err := json.Marshal(message)
		if err != nil || total+len(data)+1 > execution.BrokerResultLimit {
			return errors.New("broker result exceeds 16 MiB")
		}
		total += len(data) + 1
		return send(execution.BrokerBridgeEvent{Type: "message", Message: data})
	})
	if errors.Is(context.Cause(r.Context()), errServerStopping) {
		_ = send(execution.BrokerBridgeEvent{Type: "failed", Error: protocol + " execution canceled because the server is shutting down"})
		return
	}
	if r.Context().Err() != nil {
		return
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded):
		_ = send(execution.BrokerBridgeEvent{Type: "failed", Error: protocol + " execution timed out"})
	case err != nil:
		_ = send(execution.BrokerBridgeEvent{Type: "failed", Error: protocol + " execution failed"})
	default:
		_ = send(execution.BrokerBridgeEvent{Type: "completed"})
	}
}
