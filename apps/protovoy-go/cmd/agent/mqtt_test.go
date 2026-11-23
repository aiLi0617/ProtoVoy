package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"protovoy.dev/protovoy/internal/execution"
)

func TestAgentMQTTQoS2PublishAndSubscribe(t *testing.T) {
	const connect = "\x10\x1b\x00\x04MQTT\x04\x02\x00\x1e\x00\x0fprotovoy-client"
	const connack = "\x20\x02\x00\x00"
	const disconnect = "\xe0\x00"
	for _, fixture := range []struct {
		mode, response string
		frames         [][2]string
	}{
		{"publish", `{"bytes":2,"published":true,"qos":2,"retained":true,"topic":"demo/a"}`, [][2]string{
			{connect, connack},
			{"\x35\x0c\x00\x06demo/a\x00\x01\x00\xff", "\x50\x02\x00\x01"},
			{"\x62\x02\x00\x01", "\x70\x02\x00\x01"},
			{disconnect, ""},
		}},
		{"subscribe", `{"payload":null,"payloadBase64":"AP8=","qos":2,"retain":true,"topic":"demo/a"}`, [][2]string{
			{connect, connack},
			{"\x82\x0b\x00\x01\x00\x06demo/a\x02", "\x35\x0c\x00\x06demo/a\x00\x07\x00\xff\x90\x03\x00\x01\x02"},
			{"\x50\x02\x00\x07", "\x3d\x0c\x00\x06demo/a\x00\x07\x00\xff"},
			{"\x50\x02\x00\x07", "\x62\x02\x00\x07"},
			{"\x70\x02\x00\x07", ""},
			{disconnect, ""},
		}},
	} {
		t.Run(fixture.mode, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			broker := make(chan error, 1)
			go func() {
				broker <- runMQTTFixture(listener, fixture.frames)
			}()
			agent := testAgent(t)
			client := &http.Client{Timeout: 5 * time.Second}
			call := func(method, path, body string) *http.Response {
				t.Helper()
				req, err := http.NewRequest(method, agent.URL+path, strings.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Authorization", "Bearer "+pairingFixture)
				response, err := client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { response.Body.Close() })
				return response
			}
			input := fmt.Sprintf(`{"id":"480b7831-08f1-4753-aef3-887ba1324922","protocolId":"mqtt","target":%q,"payload":{"type":"raw","value":{"mode":%q,"topic":"demo/a","qos":2,"retain":true,"payload":"AP8=","encoding":"base64"}}}`, listener.Addr().String(), fixture.mode)
			started := call("POST", "/v1/executions", input)
			var response struct{ ExecutionID string }
			if started.StatusCode != http.StatusAccepted || json.NewDecoder(started.Body).Decode(&response) != nil {
				t.Fatalf("MQTT execution rejected: HTTP %d", started.StatusCode)
			}
			events := call("GET", "/v1/executions/"+response.ExecutionID+"/events", "")
			reader := bufio.NewScanner(events.Body)
			var data []byte
			completed := false
			for reader.Scan() {
				event := readTestEvent(t, reader.Text())
				switch event["type"] {
				case "response_chunk":
					if chunk, ok := event["data_base64"].(string); ok {
						data, err = base64.StdEncoding.AppendDecode(data, []byte(chunk))
						if err != nil {
							t.Fatal(err)
						}
					}
				case "completed":
					completed = true
				case "failed":
					t.Logf("execution failed: %v", event["message"])
				}
			}
			if err := <-broker; err != nil {
				t.Fatal(err)
			}
			if !completed || reader.Err() != nil || !bytes.Equal(data, []byte(fixture.response+"\n")) {
				t.Fatalf("unexpected MQTT result: %s, %v", data, reader.Err())
			}
		})
	}
}

func TestMQTTCancelClosesIdleSubscription(t *testing.T) {
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := listener.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	request, err := execution.PrepareMQTT(listener.Addr().String(), execution.MQTTOptions{
		Mode: "subscribe", Topic: "#", CleanSession: true, ReceiveLimit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- request.Execute(ctx, func() error { return nil }, func(any) error {
			return errors.New("unexpected message")
		})
	}()
	conn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	for _, frame := range [][2]string{
		{"\x10\x1b\x00\x04MQTT\x04\x02\x00\x00\x00\x0fprotovoy-client", "\x20\x02\x00\x00"},
		{"\x82\x06\x00\x01\x00\x01#\x00", "\x90\x03\x00\x01\x00"},
	} {
		got := make([]byte, len(frame[0]))
		if _, err := io.ReadFull(conn, got); err != nil || string(got) != frame[0] {
			t.Fatalf("unexpected MQTT packet: %x (%v)", got, err)
		}
		if _, err := io.WriteString(conn, frame[1]); err != nil {
			t.Fatal(err)
		}
	}
	cancel()
	if _, err := conn.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("cancellation did not close the broker connection: %v", err)
	}
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected cancellation result: %v", err)
	}
}

func runMQTTFixture(listener net.Listener, frames [][2]string) error {
	_ = listener.(*net.TCPListener).SetDeadline(time.Now().Add(5 * time.Second))
	conn, err := listener.Accept()
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	for _, frame := range frames {
		got := make([]byte, len(frame[0]))
		if _, err := io.ReadFull(conn, got); err != nil || string(got) != frame[0] {
			return fmt.Errorf("unexpected MQTT fixture packet: %x (%v)", got, err)
		}
		if frame[1] != "" {
			if _, err := io.WriteString(conn, frame[1]); err != nil {
				return err
			}
		}
	}
	return nil
}
