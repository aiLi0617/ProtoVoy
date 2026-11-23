package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"protovoy.dev/protovoy/internal/agentclient"
	"protovoy.dev/protovoy/internal/execution"
)

func TestAgentGRPCDescriptorJSON(t *testing.T) {
	descriptor, err := proto.Marshal(&descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{{
			Name: proto.String("echo.proto"), Package: proto.String("demo"), Syntax: proto.String("proto3"),
			MessageType: []*descriptorpb.DescriptorProto{{
				Name: proto.String("Message"),
				Field: []*descriptorpb.FieldDescriptorProto{{
					Name: proto.String("value"), Number: proto.Int32(1),
					Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					Type:  descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
				}},
			}},
			Service: []*descriptorpb.ServiceDescriptorProto{{
				Name: proto.String("Echo"),
				Method: []*descriptorpb.MethodDescriptorProto{{
					Name: proto.String("Exchange"), InputType: proto.String(".demo.Message"),
					OutputType: proto.String(".demo.Message"), ClientStreaming: proto.Bool(true),
				}},
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var calls, v1Calls, alphaCalls atomic.Int32
	var refuseReflection atomic.Bool
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("Trailer", "Grpc-Status")
		if r.Header.Get("X-Test") != "reflection-fixture" {
			t.Error("gRPC metadata was not forwarded")
		}
		switch r.URL.Path {
		case "/grpc.reflection.v1.ServerReflection/ServerReflectionInfo",
			"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo":
			symbol := append([]byte{0, 0, 0, 0, 11, 34, 9}, []byte("demo.Echo")...)
			if err != nil || !bytes.Equal(data, symbol) {
				t.Errorf("invalid reflection request: %x, %v", data, err)
			}
			if r.URL.Path == "/grpc.reflection.v1.ServerReflection/ServerReflectionInfo" {
				v1Calls.Add(1)
				w.Header().Set("Grpc-Status", "12")
				return
			}
			alphaCalls.Add(1)
			if refuseReflection.Load() {
				w.Header().Set("Grpc-Status", "12")
				return
			}
			response := protowire.AppendBytes(protowire.AppendTag(nil, 4, protowire.BytesType), descriptor)
			_, _ = w.Write(append(binary.BigEndian.AppendUint32([]byte{0}, uint32(len(response))), response...))
			w.Header().Set("Grpc-Status", "0")
			return
		case "/demo.Echo/Exchange":
			calls.Add(1)
		default:
			t.Errorf("unexpected gRPC path %q", r.URL.Path)
		}
		if want := []byte{0, 0, 0, 0, 2, 8, 1, 0, 0, 0, 0, 2, 8, 2}; err != nil || !bytes.Equal(data, want) {
			t.Errorf("unexpected ProtoJSON request frames: %x, %v", data, err)
		}
		_, _ = w.Write([]byte{0, 0, 0, 0, 2, 8, 3})
		w.Header().Set("Grpc-Status", "0")
	}))
	upstream.Config.Protocols = new(http.Protocols)
	upstream.Config.Protocols.SetUnencryptedHTTP2(true)
	upstream.Start()
	defer upstream.Close()

	agent := testAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := agentclient.Pair(ctx, agent.Listener.Addr().String(), pairingFixture, agent.Client().Do)
	if err != nil {
		t.Fatal(err)
	}
	request := execution.GRPCRequest{
		Target: upstream.URL, Service: "demo.Echo", Method: "Exchange", Mode: "client_streaming",
		MessageJSON: `[{"value":1},{"value":2}]`, DescriptorSetBase64: base64.StdEncoding.EncodeToString(descriptor),
		Metadata: [][2]string{{"x-test", "reflection-fixture"}},
	}
	var messages []execution.GRPCMessage
	onMessage := func(message execution.GRPCMessage) { messages = append(messages, message) }
	if err := client.RunGRPC(ctx, request, func(int) {}, onMessage); err != nil || len(messages) != 1 ||
		messages[0].JSONPreview != `{"value":3}` || !bytes.Equal(messages[0].Preview, []byte{8, 3}) {
		t.Fatalf("unexpected gRPC response: %+v, %v", messages, err)
	}
	request.MessageJSON = `[{"unknown":1}]`
	if err := client.RunGRPC(ctx, request, func(int) {}, onMessage); err == nil ||
		calls.Load() != 1 || v1Calls.Load() != 0 || alphaCalls.Load() != 0 {
		t.Fatal("explicit descriptors should skip reflection and reject invalid ProtoJSON before sending")
	}
	request.DescriptorSetBase64 = ""
	request.MessageJSON = `[{"value":1},{"value":2}]`
	if err := client.RunGRPC(ctx, request, func(int) {}, onMessage); err != nil || len(messages) != 2 ||
		messages[1].JSONPreview != `{"value":3}` || calls.Load() != 2 || v1Calls.Load() != 1 || alphaCalls.Load() != 1 {
		t.Fatalf("v1alpha fallback did not resolve the method: %+v, %v", messages, err)
	}
	refuseReflection.Store(true)
	if err := client.RunGRPC(ctx, request, func(int) {}, onMessage); !errors.Is(err, execution.ErrGRPCReflection) || calls.Load() != 2 {
		t.Fatalf("failed reflection should prevent execution: %v", err)
	}
}

func TestAgentGRPCEmitsMessageBoundaries(t *testing.T) {
	message := bytes.Repeat([]byte{0, 1, 255}, 4000)
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("Trailer", "Grpc-Status")
		_, _ = w.Write(append(binary.BigEndian.AppendUint32([]byte{0}, uint32(len(message))), message...))
		_, _ = w.Write([]byte{0, 0, 0, 0, 0})
		w.Header().Set("Grpc-Status", "0")
	}))
	upstream.Config.Protocols = new(http.Protocols)
	upstream.Config.Protocols.SetUnencryptedHTTP2(true)
	upstream.Start()
	defer upstream.Close()
	agent := testAgent(t)
	client := &http.Client{Timeout: 5 * time.Second}
	call := func(method, path, body string) *http.Response {
		t.Helper()
		request, err := http.NewRequest(method, agent.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+pairingFixture)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { response.Body.Close() })
		return response
	}
	input := `{"id":"480b7831-08f1-4753-aef3-887ba1324922","protocolId":"grpc","target":"` + upstream.URL + `","payload":{"type":"grpc","service":"demo.Echo","method":"Watch","messageBase64":"CAE=","mode":"server_streaming"}}`
	started := call("POST", "/v1/executions", input)
	var response struct{ ExecutionID string }
	if started.StatusCode != 202 || json.NewDecoder(started.Body).Decode(&response) != nil || !validID(response.ExecutionID) {
		t.Fatalf("could not start gRPC fixture: HTTP %d", started.StatusCode)
	}
	stream := call("GET", "/v1/executions/"+response.ExecutionID+"/events", "")
	reader := bufio.NewScanner(stream.Body)
	var bodies [][]byte
	var data []byte
	completed := false
	for reader.Scan() {
		event := readTestEvent(t, reader.Text())
		switch event["type"] {
		case "response_chunk":
			if encoded, ok := event["data_base64"].(string); ok {
				chunk, err := base64.StdEncoding.DecodeString(encoded)
				if err != nil || event["messageIndex"] != float64(len(bodies)+1) {
					t.Fatal("invalid gRPC chunk")
				}
				data = append(data, chunk...)
				if event["messageEnd"] == true {
					bodies = append(bodies, data)
					data = nil
				}
			}
		case "completed":
			completed = event["summary"].(map[string]any)["protocolId"] == "grpc"
		}
	}
	if reader.Err() != nil || !completed || len(bodies) != 2 || !bytes.Equal(bodies[0], message) || len(bodies[1]) != 0 {
		t.Fatalf("gRPC event stream lost response messages: %v", reader.Err())
	}
}
