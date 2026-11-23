package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"protovoy.dev/protovoy/internal/execution"
)

func TestAMQPBridgeRejectsBrowserBrokerAndCredentialOverrides(t *testing.T) {
	const origin = "https://workbench.example.com"
	access, err := configureWebAccess("127.0.0.1:39300", origin, "test-cert", "test-key", "operator", "fixture-password-for-tests")
	if err != nil {
		t.Fatal(err)
	}
	api := handler(access, t.TempDir(), "", "", socketTargets{
		AMQP: &amqpEndpoint{target: "amqps://broker.example.com/jobs"},
	})
	for _, tc := range []struct {
		origin, marker, body string
		auth                 bool
		want                 int
	}{
		{origin, "1", "{}", false, http.StatusUnauthorized},
		{"https://other.example.com", "1", "{}", true, http.StatusForbidden},
		{origin, "", "{}", true, http.StatusForbidden},
		{origin, "1", `{"expectedTarget":"amqps://broker.example.com/other"}`, true, http.StatusConflict},
		{origin, "1", `{"expectedTarget":"amqps://other.example.com/jobs"}`, true, http.StatusConflict},
		{origin, "1", `{"mode":"publish","username":"other","password":"other"}`, true, http.StatusBadRequest},
		{origin, "1", `{"mode":"publish","caPem":"other"}`, true, http.StatusBadRequest},
		{origin, "1", `{"mode":"consume"}`, true, http.StatusBadRequest},
	} {
		r := httptest.NewRequest(http.MethodPost, origin+execution.AMQPBridgePath, strings.NewReader(tc.body))
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("X-ProtoVoy-Preview", tc.marker)
		if tc.auth {
			r.SetBasicAuth("operator", "fixture-password-for-tests")
		}
		w := httptest.NewRecorder()
		api.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("AMQP bridge returned %d, want %d for %q", w.Code, tc.want, tc.body)
		}
	}
}

func TestAMQPBridgePublishesThroughConfiguredBroker(t *testing.T) {
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_ = listener.SetDeadline(time.Now().Add(5 * time.Second))
	brokerResult := make(chan error, 1)
	go func() { brokerResult <- amqpPublishPeer(listener) }()
	target := "amqp://" + listener.Addr().String() + "/jobs"
	broker, err := execution.NewAMQPBroker(target, "writer", "fixture", "", "")
	if err != nil {
		t.Fatal(err)
	}
	const addr = "127.0.0.1:39300"
	const origin = "http://" + addr
	api := handler(loopbackAccess(addr), t.TempDir(), "", "", socketTargets{AMQP: &amqpEndpoint{target: target, broker: broker}})
	r := httptest.NewRequest(http.MethodPost, origin+execution.AMQPBridgePath, strings.NewReader(
		`{"mode":"publish","routingKey":"jobs","payload":"AP8=","encoding":"base64"}`))
	r.Header.Set("Origin", origin)
	r.Header.Set("X-ProtoVoy-Preview", "1")
	w := httptest.NewRecorder()
	api.ServeHTTP(w, r)
	if err := <-brokerResult; err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(w.Body)
	var connected, message, terminal execution.BrokerBridgeEvent
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/x-ndjson" ||
		decoder.Decode(&connected) != nil || decoder.Decode(&message) != nil || decoder.Decode(&terminal) != nil ||
		connected.Type != "connected" || message.Type != "message" || terminal.Type != "completed" ||
		string(message.Message) != `{"bytes":2,"confirmation":"ack","exchange":"","published":true,"routingKey":"jobs"}` {
		t.Fatalf("unexpected AMQP bridge events: %v, %s, %v", connected, message.Message, terminal)
	}
}

func amqpPublishPeer(listener net.Listener) error {
	conn, err := listener.Accept()
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	var protocol [8]byte
	if _, err := io.ReadFull(conn, protocol[:]); err != nil || string(protocol[:]) != "AMQP\x00\x00\x09\x01" {
		return fmt.Errorf("invalid AMQP protocol header (%v)", err)
	}
	send := func(channel byte, payload string) error {
		frame := binary.BigEndian.AppendUint32([]byte{1, 0, channel}, uint32(len(payload)))
		_, err := conn.Write(append(append(frame, payload...), 0xce))
		return err
	}
	if err := send(0, "\x00\x0a\x00\x0a\x00\x09\x00\x00\x00\x00\x00\x00\x00\x05PLAIN\x00\x00\x00\x05en_US"); err != nil {
		return err
	}
	for _, step := range []struct {
		kind, channel    byte
		prefix, contains string
		reply            string
	}{
		{1, 0, "\x00\x0a\x00\x0b", "\x00writer\x00fixture", "\x00\x0a\x00\x1e\x00\x01\x00\x02\x00\x00\x00\x00"},
		{1, 0, "\x00\x0a\x00\x1f", "", ""},
		{1, 0, "\x00\x0a\x00\x28\x04jobs", "", "\x00\x0a\x00\x29\x00"},
		{1, 1, "\x00\x14\x00\x0a", "", "\x00\x14\x00\x0b\x00\x00\x00\x00"},
		{1, 1, "\x00\x55\x00\x0a", "", "\x00\x55\x00\x0b"},
		{1, 1, "\x00\x3c\x00\x28\x00\x00\x00\x04jobs\x01", "", ""},
		{2, 1, "\x00\x3c", "application/octet-stream", ""},
		{3, 1, "\x00\xff", "", "\x00\x3c\x00\x50\x00\x00\x00\x00\x00\x00\x00\x01\x00"},
		{1, 0, "\x00\x0a\x00\x32", "", "\x00\x0a\x00\x33"},
	} {
		var frame [7]byte
		if _, err := io.ReadFull(conn, frame[:]); err != nil {
			return err
		}
		size := binary.BigEndian.Uint32(frame[3:])
		if frame[0] != step.kind || frame[1] != 0 || frame[2] != step.channel || size > 4096 {
			return fmt.Errorf("unexpected AMQP fixture frame header: %x", frame)
		}
		payload := make([]byte, int(size)+1)
		if _, err := io.ReadFull(conn, payload); err != nil {
			return err
		}
		if !bytes.HasPrefix(payload, []byte(step.prefix)) || !bytes.Contains(payload, []byte(step.contains)) {
			return fmt.Errorf("unexpected AMQP fixture method or content for %x", step.prefix)
		}
		if step.reply != "" {
			if err := send(step.channel, step.reply); err != nil {
				return err
			}
		}
	}
	return nil
}
