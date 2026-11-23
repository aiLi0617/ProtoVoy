package main

import (
	"net"
	"testing"
	"time"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

func TestScheduledAMQPUsesConfiguredBrokerAndPublicationConfirmation(t *testing.T) {
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	listener.SetDeadline(time.Now().Add(5 * time.Second))
	done := make(chan error, 1)
	go func() { done <- amqpPublishPeer(listener) }()
	target := "amqp://" + listener.Addr().String() + "/jobs"
	broker, err := execution.NewAMQPBroker(target, "writer", "fixture", "", "")
	if err != nil {
		t.Fatal(err)
	}
	execute := scheduledAMQPExecutor(&amqpEndpoint{target: target, broker: broker})
	request := project.Request{URL: "{{broker}}", Body: "AP8=", Variables: map[string]string{"broker": target, "queue": "jobs"}, BrokerTimeoutMS: 3000,
		AMQP: &project.AMQPSettings{Mode: "publish", RoutingKey: "{{queue}}", Encoding: "base64", ExchangeType: "direct", ReceiveLimit: 1}}
	for _, change := range []func(*project.Request){
		func(r *project.Request) { r.URL = "amqp://other.test/jobs" },
		func(r *project.Request) { r.AMQP.PasswordRef = "wrong" },
		func(r *project.Request) { r.BrokerTimeoutMS = 300001 },
	} {
		input := request
		settings := *request.AMQP
		input.AMQP = &settings
		change(&input)
		if result, err := execute(t.Context(), input); err == nil || result.Success {
			t.Fatal("invalid AMQP request accepted")
		}
	}
	if result, err := execute(t.Context(), request); err != nil || !result.Success {
		t.Fatalf("publish: %+v %v", result, err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("AMQP peer did not finish")
	}
}
