package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"protovoy.dev/protovoy/internal/execution"
)

func TestMQTTBridgeRequiresHostedAccessAndFixedBroker(t *testing.T) {
	const origin = "https://workbench.example.com"
	access, err := configureWebAccess("127.0.0.1:39300", origin, "test-cert", "test-key", "operator", "fixture-password-for-tests")
	if err != nil {
		t.Fatal(err)
	}
	api := handler(access, t.TempDir(), "", "", socketTargets{MQTT: &mqttEndpoint{target: "mqtts://broker.example.com"}})
	for _, tc := range []struct {
		name, origin, marker, body string
		authenticate               bool
		status                     int
	}{
		{"anonymous", origin, "1", "{}", false, http.StatusUnauthorized},
		{"cross origin", "https://other.example.com", "1", "{}", true, http.StatusForbidden},
		{"missing marker", origin, "", "{}", true, http.StatusForbidden},
		{"alternate broker", origin, "1", `{"expectedTarget":"mqtt://other.example.com"}`, true, http.StatusConflict},
		{"credential override", origin, "1", `{"username":"other","password":"other"}`, true, http.StatusBadRequest},
		{"session override", origin, "1", `{"clientId":"other","cleanSession":false}`, true, http.StatusBadRequest},
		{"trailing JSON", origin, "1", `{} {}`, true, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, origin+execution.MQTTBridgePath, strings.NewReader(tc.body))
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("X-ProtoVoy-Preview", tc.marker)
			if tc.authenticate {
				r.SetBasicAuth("operator", "fixture-password-for-tests")
			}
			w := httptest.NewRecorder()
			api.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("MQTT bridge returned %d, want %d", w.Code, tc.status)
			}
		})
	}
}

func TestMQTTBridgeStreamsAndCancelsVerifiedBrokerConnection(t *testing.T) {
	tlsFixture := httptest.NewTLSServer(http.NotFoundHandler())
	defer tlsFixture.Close()
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_ = listener.SetDeadline(time.Now().Add(5 * time.Second))
	brokerListener := tls.NewListener(listener, &tls.Config{Certificates: tlsFixture.TLS.Certificates, MinVersion: tls.VersionTLS12})
	target := "mqtts://" + listener.Addr().String()
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: tlsFixture.Certificate().Raw})
	broker, err := execution.NewMQTTBroker(target, "example.com", string(ca))
	if err != nil {
		t.Fatal(err)
	}
	brokerResult := make(chan error, 1)
	go func() {
		brokerResult <- exerciseMQTTSubscription(brokerListener)
	}()

	server := httptest.NewUnstartedServer(nil)
	addr := server.Listener.Addr().String()
	server.Config.Handler = handler(loopbackAccess(addr), t.TempDir(), "", "", socketTargets{
		MQTT: &mqttEndpoint{target: target, broker: broker, username: "reader", password: "fixture"},
	})
	server.Start()
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	input := `{"mode":"subscribe","topic":"demo/a","receiveLimit":2}`
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+execution.MQTTBridgePath, strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", server.URL)
	request.Header.Set("X-ProtoVoy-Preview", "1")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	decoder := json.NewDecoder(response.Body)
	var connected, message execution.BrokerBridgeEvent
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "application/x-ndjson" ||
		decoder.Decode(&connected) != nil || decoder.Decode(&message) != nil ||
		connected.Type != "connected" || message.Type != "message" ||
		string(message.Message) != `{"payload":null,"payloadBase64":"AP8=","qos":0,"retain":true,"topic":"demo/a"}` {
		t.Fatalf("unexpected MQTT stream: HTTP %d, %v, %s", response.StatusCode, connected, message.Message)
	}
	cancel()
	if err := <-brokerResult; err != nil {
		t.Fatal(err)
	}
}

func exerciseMQTTSubscription(listener net.Listener) error {
	return exerciseMQTTSubscriptionEnd(listener, false)
}

func exerciseMQTTSubscriptionEnd(listener net.Listener, graceful bool) error {
	conn, err := listener.Accept()
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	packet := make([]byte, 54)
	if _, err := io.ReadFull(conn, packet); err != nil {
		return err
	}
	if !bytes.Equal(packet[:14], []byte("\x10\x34\x00\x04MQTT\x04\xc2\x00\x0f\x00\x17")) ||
		!bytes.HasPrefix(packet[14:37], []byte("pv")) ||
		!bytes.Equal(packet[37:], []byte("\x00\x06reader\x00\x07fixture")) {
		return errors.New("broker did not receive operator credentials and an isolated session")
	}
	if _, err := io.WriteString(conn, "\x20\x02\x00\x00"); err != nil {
		return err
	}
	packet = packet[:13]
	if _, err := io.ReadFull(conn, packet); err != nil {
		return err
	}
	if string(packet) != "\x82\x0b\x00\x01\x00\x06demo/a\x00" {
		return errors.New("broker received an unexpected subscription")
	}
	if _, err := io.WriteString(conn, "\x90\x03\x00\x01\x00\x31\x0a\x00\x06demo/a\x00\xff"); err != nil {
		return err
	}
	if graceful {
		if _, err := io.ReadFull(conn, packet[:2]); err != nil || !bytes.Equal(packet[:2], []byte{0xe0, 0}) {
			return errors.New("completed subscription did not send MQTT DISCONNECT")
		}
	}
	if _, err := conn.Read(packet); !errors.Is(err, io.EOF) {
		return errors.New("cancelling the HTTP stream did not close the broker connection")
	}
	return nil
}
