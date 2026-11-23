//go:build !js

package execution

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestGRPCReflectionCancellationStopsFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/grpc.reflection.v1.ServerReflection/ServerReflectionInfo" {
			t.Errorf("unexpected gRPC path %q", r.URL.Path)
		}
		cancel()
		<-r.Context().Done()
	}))
	server.Config.Protocols = new(http.Protocols)
	server.Config.Protocols.SetUnencryptedHTTP2(true)
	server.Start()
	defer server.Close()
	err := (GRPCClient{}).Execute(ctx, GRPCRequest{
		Target: server.URL, Service: "demo.Echo", Method: "Watch", MessageJSON: "{}",
		TimeoutMS: 3000,
	}, func(int, http.Header) error {
		t.Error("reflection metadata leaked into the execution response")
		return nil
	}, func([]byte, string) error {
		t.Error("reflection descriptors leaked into the execution response")
		return nil
	})
	if !errors.Is(err, context.Canceled) || calls.Load() != 1 {
		t.Fatalf("reflection ignored cancellation: %d calls, %v", calls.Load(), err)
	}
}

func TestGRPCUsesHTTP2AndChecksStatusTrailers(t *testing.T) {
	for _, tls := range []bool{false, true} {
		name := "cleartext HTTP2"
		if tls {
			name = "verified TLS HTTP2"
		}
		t.Run(name, func(t *testing.T) {
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				request, err := io.ReadAll(r.Body)
				want := []byte{0, 0, 0, 0, 2, 0x08, 1, 0, 0, 0, 0, 2, 0x08, 2}
				if err != nil || !bytes.Equal(request, want) || r.ProtoMajor != 2 || r.Method != http.MethodPost ||
					r.URL.Path != "/demo.Echo/Exchange" || r.Header.Get("TE") != "trailers" ||
					r.Header.Get("Example-Bin") != "AAE" || r.Header.Get("Grpc-Timeout") == "" {
					t.Error("incorrect gRPC HTTP/2 request")
				}
				w.Header().Set("Content-Type", "application/grpc+proto")
				w.Header().Set("Trailer", "Grpc-Status")
				for _, part := range [][]byte{{0, 0}, {0, 0, 2, 0x08}, {3, 0, 0, 0, 0, 0}} {
					_, _ = w.Write(part)
					w.(http.Flusher).Flush()
				}
				w.Header().Set("Grpc-Status", "7")
			}))
			client := GRPCClient{}
			if tls {
				server.EnableHTTP2 = true
				server.StartTLS()
				client.Transport = server.Client().Transport
			} else {
				server.Config.Protocols = new(http.Protocols)
				server.Config.Protocols.SetUnencryptedHTTP2(true)
				server.Start()
			}
			defer server.Close()
			var replies [][]byte
			err := client.Execute(context.Background(), GRPCRequest{
				Target: server.URL, Service: "demo.Echo", Method: "Exchange", Mode: "bidi_streaming",
				MessageBase64: "CAE=\nCAI=", Metadata: [][2]string{{"example-bin", "AAE="}}, TimeoutMS: 3000,
			}, func(status int, _ http.Header) error {
				if status != 200 {
					t.Errorf("HTTP status %d", status)
				}
				return nil
			}, func(data []byte, _ string) error {
				replies = append(replies, data)
				return nil
			})
			var status GRPCStatusError
			if !errors.As(err, &status) || status.Code != 7 || len(replies) != 2 ||
				!bytes.Equal(replies[0], []byte{0x08, 3}) || len(replies[1]) != 0 {
				t.Fatalf("gRPC replies or trailer lost: %+v, %v", replies, err)
			}
		})
	}
}

func TestGRPCRejectsMalformedFrames(t *testing.T) {
	tooLarge := binary.BigEndian.AppendUint32([]byte{0}, GRPCMessageLimit+1)
	for _, frame := range [][]byte{{0, 0}, {0, 0, 0, 0, 2, 1}, {1, 0, 0, 0, 0}, tooLarge} {
		if _, err := readGRPCMessages(bytes.NewReader(frame), true, grpcResponseLimit, func([]byte) error { return nil }); err == nil {
			t.Fatal("accepted malformed gRPC frame")
		}
	}
}
