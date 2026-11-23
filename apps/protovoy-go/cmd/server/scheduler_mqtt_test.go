package main

import (
	"crypto/tls"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

func TestScheduledMQTTUsesServerCredentialsAndIsolatedSession(t *testing.T) {
	fixture := httptest.NewTLSServer(http.NotFoundHandler())
	defer fixture.Close()
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	listener.SetDeadline(time.Now().Add(5 * time.Second))
	secure := tls.NewListener(listener, &tls.Config{Certificates: fixture.TLS.Certificates, MinVersion: tls.VersionTLS12})
	target := "mqtts://" + listener.Addr().String()
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: fixture.Certificate().Raw})
	broker, err := execution.NewMQTTBroker(target, "example.com", string(ca))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- exerciseMQTTSubscriptionEnd(secure, true) }()
	execute := scheduledMQTTExecutor(&mqttEndpoint{target: target, broker: broker, username: "reader", password: "fixture", passwordRef: "broker"})
	request := project.Request{URL: "{{broker}}", Variables: map[string]string{"broker": target, "topic": "demo/a"}, BrokerTimeoutMS: 3000,
		MQTT: &project.MQTTSettings{Mode: "subscribe", Topic: "{{topic}}", QoS: 0, ReceiveLimit: 1, PasswordRef: "broker"}}
	for _, change := range []func(*project.Request){
		func(r *project.Request) { r.URL = "mqtts://other.test" },
		func(r *project.Request) { r.MQTT.PasswordRef = "wrong" },
		func(r *project.Request) { r.BrokerTimeoutMS = 300001 },
	} {
		input := request
		settings := *request.MQTT
		input.MQTT = &settings
		change(&input)
		if result, err := execute(t.Context(), input); err == nil || result.Success {
			t.Fatal("invalid MQTT request accepted")
		}
	}
	if result, err := execute(t.Context(), request); err != nil || !result.Success {
		t.Fatalf("subscribe: %+v %v", result, err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("MQTT peer did not finish")
	}
}
