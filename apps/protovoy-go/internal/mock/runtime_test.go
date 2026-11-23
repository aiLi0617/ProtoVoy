//go:build !js

package mock

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestMockStopCancelsDelayedRequest(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	runtime, err := Start(ctx, "127.0.0.1:0", "test", []byte(`[{"id":"delayed","method":"GET","path":"/slow","status":200,"delayMs":30000}]`))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(ctx)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+runtime.Address+"/m1/default-project/default/slow", nil)
		response, err := http.DefaultClient.Do(request)
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			response.Body.Close()
		}
	}()
	for {
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+runtime.Address+"/health", nil)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		var health struct{ RequestCount int }
		err = json.NewDecoder(response.Body).Decode(&health)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if health.RequestCount > 0 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("delayed request did not reach Mock")
		case <-time.After(time.Millisecond):
		}
	}
	if err := runtime.Close(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal("delayed request remained active after stop")
	}
}

func TestEmbeddedMockLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	runtime, err := Start(ctx, "127.0.0.1:0", "test", []byte(`[{"id":"fixture","method":"GET","path":"/hello","status":201,"body":"runtime response"}]`))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(ctx)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+runtime.Address+"/m1/default-project/default/hello", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 201 || string(data) != "runtime response" {
		t.Fatalf("unexpected mock response: %d %q %v", response.StatusCode, data, err)
	}
	if err := runtime.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if response, err := http.DefaultClient.Do(request); err == nil {
		response.Body.Close()
		t.Fatal("Mock listener remained active after stopping")
	}
}
