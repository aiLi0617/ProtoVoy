package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"protovoy.dev/protovoy/internal/execution"
)

func TestGRPCBridgeStopsAfterPartialStream(t *testing.T) {
	t.Run("timeout", func(t *testing.T) { testGRPCBridgePartialStream(t, false) })
	t.Run("shutdown", func(t *testing.T) { testGRPCBridgePartialStream(t, true) })
}

func testGRPCBridgePartialStream(t *testing.T, shutdown bool) {
	t.Helper()
	started := make(chan struct{})
	cancelled := make(chan struct{}, 1)
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Grpc-Timeout") == "" {
			t.Error("gRPC timeout metadata was missing")
		}
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("Trailer", "Grpc-Status")
		_, _ = w.Write([]byte{0, 0, 0, 0, 1, 255})
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
		cancelled <- struct{}{}
	}))
	upstream.Config.Protocols = new(http.Protocols)
	upstream.Config.Protocols.SetUnencryptedHTTP2(true)
	upstream.Start()
	defer upstream.Close()
	const addr = "127.0.0.1:39300"
	api := handler(loopbackAccess(addr), t.TempDir(), "", "", socketTargets{GRPC: upstream.URL})
	r := httptest.NewRequest(http.MethodPost, "http://"+addr+execution.GRPCBridgePath,
		strings.NewReader(`{"service":"demo.Echo","method":"Watch","mode":"server_streaming","timeoutMs":500}`))
	r.Header.Set("Origin", "http://"+addr)
	r.Header.Set("X-ProtoVoy-Preview", "1")
	w := httptest.NewRecorder()
	expectedEvents := []string{"status", "message", "failed"}
	if shutdown {
		done := make(chan struct{})
		go func() {
			defer close(done)
			select {
			case <-started:
				api.bridge.stop()
			case <-t.Context().Done():
			}
		}()
		api.ServeHTTP(w, r)
		<-done
		select {
		case <-cancelled:
		case <-time.After(time.Second):
			t.Fatal("upstream stream was not cancelled")
		}
		if strings.Contains(w.Body.String(), `"code":"timeout"`) {
			t.Fatal("shutdown waited for the gRPC deadline")
		}
		if !strings.Contains(w.Body.String(), `"type":"failed"`) ||
			!strings.Contains(w.Body.String(), "gRPC execution canceled because the server is shutting down") {
			t.Fatalf("missing shutdown event: %s", w.Body.String())
		}
		return
	}
	api.ServeHTTP(w, r)
	decoder := json.NewDecoder(w.Body)
	for _, expected := range expectedEvents {
		var event execution.GRPCBridgeEvent
		if err := decoder.Decode(&event); err != nil || event.Type != expected || expected == "failed" && event.Code != "timeout" {
			t.Fatalf("unexpected gRPC event: %+v (%v)", event, err)
		}
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("upstream stream was not cancelled")
	}
}

func TestGRPCBridgeEnforcesOriginAndTarget(t *testing.T) {
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/demo.Echo/Watch" || r.ProtoMajor != 2 {
			t.Error("unexpected upstream gRPC request")
		}
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("Trailer", "Grpc-Status")
		_, _ = w.Write([]byte{0, 0, 0, 0, 1, 255})
		w.Header().Set("Grpc-Status", "7")
	}))
	upstream.Config.Protocols = new(http.Protocols)
	upstream.Config.Protocols.SetUnencryptedHTTP2(true)
	upstream.Start()
	defer upstream.Close()
	const addr = "127.0.0.1:39300"
	const origin = "http://" + addr
	api := handler(loopbackAccess(addr), t.TempDir(), "", "", socketTargets{GRPC: upstream.URL})
	for _, test := range []struct {
		origin, marker, target string
		want                   int
	}{
		{"https://other.test", "1", "", 403},
		{origin, "", "", 403},
		{origin, "1", "http://other.test", 409},
		{origin, "1", upstream.URL, 200},
	} {
		input, _ := json.Marshal(execution.GRPCRequest{Target: test.target, Service: "demo.Echo", Method: "Watch"})
		r := httptest.NewRequest(http.MethodPost, origin+execution.GRPCBridgePath, strings.NewReader(string(input)))
		r.Header.Set("Origin", test.origin)
		r.Header.Set("X-ProtoVoy-Preview", test.marker)
		w := httptest.NewRecorder()
		api.ServeHTTP(w, r)
		if w.Code != test.want {
			t.Fatalf("HTTP %d, want %d", w.Code, test.want)
		}
		if test.want == 200 {
			decoder := json.NewDecoder(w.Body)
			var status, message, terminal execution.GRPCBridgeEvent
			if decoder.Decode(&status) != nil || decoder.Decode(&message) != nil || decoder.Decode(&terminal) != nil ||
				status.Type != "status" || status.Status != 200 || message.Type != "message" || message.Size != 1 ||
				string(message.Preview) != "\xff" || terminal.Type != "failed" || terminal.Error != "gRPC status 7" {
				t.Fatal("bridge lost gRPC response or terminal status")
			}
		}
	}
}
