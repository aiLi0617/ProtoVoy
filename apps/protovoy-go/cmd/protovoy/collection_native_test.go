package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

func TestCollectionRunsSocketsAndReportsGRPCTrailerError(t *testing.T) {
	grpc := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil || !bytes.Equal(body, []byte{0, 0, 0, 0, 2, 8, 7}) || r.URL.Path != "/demo.Echo/Ping" {
			t.Error("unexpected gRPC request")
		}
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("Trailer", "Grpc-Status")
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Grpc-Status", "7")
	}))
	grpc.Config.Protocols = new(http.Protocols)
	grpc.Config.Protocols.SetUnencryptedHTTP2(true)
	grpc.Start()
	defer grpc.Close()

	tcp, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tcp.Close()
	tcpData := make(chan string, 1)
	go func() {
		conn, err := tcp.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		var message [3]byte
		if _, err := io.ReadFull(conn, message[:]); err == nil {
			tcpData <- string(message[:])
			_, _ = conn.Write([]byte("ok"))
		}
	}()

	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	udpData := make(chan []byte, 1)
	go func() {
		buffer := make([]byte, 32)
		n, peer, err := udp.ReadFrom(buffer)
		if err == nil {
			udpData <- buffer[:n]
			_, _ = udp.WriteTo([]byte("ok"), peer)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var report bytes.Buffer
	err = runCollection(ctx, project.Project{
		CollectionVariables: map[string]string{"text": "abc", "payload": "CAc="},
		Requests: []project.Request{
			{Name: "TCP", URL: tcp.Addr().String(), Socket: &execution.SocketPayload{
				Network: "tcp", Framing: "fixed", FixedLength: 2, Data: "{{text}}",
			}},
			{Name: "gRPC", URL: grpc.URL, Body: "{{payload}}", GRPC: &project.GRPCSettings{
				Service: "demo.Echo", Method: "Ping",
			}},
			{Name: "UDP", URL: udp.LocalAddr().String(), Socket: &execution.SocketPayload{
				Network: "udp", Encoding: "base64", Data: "AP8=",
			}},
		},
	}, collectionRunOptions{}, &report)
	if err == nil || err.Error() != "1 collection request(s) failed" {
		t.Fatalf("unexpected collection error: %v", err)
	}
	decoder := json.NewDecoder(&report)
	for _, want := range []collectionResult{
		{Name: "TCP", Protocol: "tcp", Passed: true, RequestIndex: 1},
		{Name: "gRPC", Protocol: "grpc", Status: 200, Error: "gRPC status 7", RequestIndex: 2},
		{Name: "UDP", Protocol: "udp", Passed: true, RequestIndex: 3},
	} {
		got := collectionResult{DurationMS: -1}
		if err := decoder.Decode(&got); err != nil || got.DurationMS < 0 {
			t.Fatalf("invalid timed result: %+v (%v)", got, err)
		}
		want.DurationMS = got.DurationMS
		if got != want {
			t.Fatalf("result = %+v, want %+v", got, want)
		}
	}
	if got := <-tcpData; got != "abc" {
		t.Fatalf("TCP payload = %q", got)
	}
	if got := <-udpData; !bytes.Equal(got, []byte{0, 255}) {
		t.Fatalf("UDP payload = %v", got)
	}
}
