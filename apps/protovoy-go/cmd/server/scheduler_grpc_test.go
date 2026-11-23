package main

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"protovoy.dev/protovoy/internal/project"
)

func TestScheduledGRPCTrailersDeadlineAndFixedTarget(t *testing.T) {
	for _, scenario := range []string{"success", "grpc error", "timeout"} {
		t.Run(scenario, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/demo.Echo/Watch" || r.ProtoMajor != 2 || r.Header.Get("X-Fixture") != "scheduled" {
					t.Error("gRPC request was not prepared correctly")
				}
				w.Header().Set("Content-Type", "application/grpc")
				w.Header().Set("Trailer", "Grpc-Status, Grpc-Message")
				w.Write([]byte{0, 0, 0, 0, 1, 255})
				if scenario == "timeout" {
					w.(http.Flusher).Flush()
					<-r.Context().Done()
					return
				}
				w.Header().Set("Grpc-Status", "0")
				if scenario == "grpc error" {
					w.Header().Set("Grpc-Status", "7")
					w.Header().Set("Grpc-Message", "secret diagnostic")
				}
			}))
			upstream.Config.Protocols = new(http.Protocols)
			upstream.Config.Protocols.SetUnencryptedHTTP2(true)
			upstream.Start()
			defer upstream.Close()
			execute := scheduledGRPCExecutor(upstream.URL)
			request := project.Request{URL: "{{target}}", Headers: map[string]string{"X-Fixture": "{{marker}}"}, Variables: map[string]string{"target": upstream.URL, "marker": "scheduled"},
				GRPC: &project.GRPCSettings{Service: "demo.Echo", Method: "Watch", Mode: "server_streaming", TimeoutMS: 200}}
			wrong := request
			wrong.URL = upstream.URL + "/unconfigured"
			if result, err := execute(t.Context(), wrong); err == nil || result.Success {
				t.Fatal("wrong target accepted")
			}
			if calls.Load() != 0 {
				t.Fatal("wrong target reached upstream")
			}
			result, err := execute(t.Context(), request)
			if scenario == "success" {
				if err != nil || !result.Success || result.Status != 200 {
					t.Fatalf("success: %+v %v", result, err)
				}
			} else if err == nil || result.Success {
				t.Fatalf("failure accepted: %+v %v", result, err)
			}
		})
	}
}
