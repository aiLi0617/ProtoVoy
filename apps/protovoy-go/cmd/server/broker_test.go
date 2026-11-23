package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBrokerStreamStopsOnShutdown(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	stopping, started, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	handler := stopStreamOnShutdown(stopping, func(w http.ResponseWriter, r *http.Request) {
		streamBroker(w, r, "fixture", func(ctx context.Context, connected func() error, _ func(any) error) error {
			if err := connected(); err != nil {
				return err
			}
			close(started)
			<-ctx.Done()
			return ctx.Err()
		}, time.Minute)
	})
	response := httptest.NewRecorder()
	go func() {
		defer close(done)
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/", nil).WithContext(ctx))
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("broker stream did not start")
	}
	close(stopping)
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("broker stream did not stop")
	}
	if ctx.Err() != nil || !strings.Contains(response.Body.String(), `"type":"connected"`) ||
		!strings.Contains(response.Body.String(), `"type":"failed","error":"fixture execution canceled because the server is shutting down"`) {
		t.Fatalf("unexpected shutdown response: %s", response.Body.String())
	}
}
