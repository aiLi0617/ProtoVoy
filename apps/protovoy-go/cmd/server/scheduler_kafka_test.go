package main

import (
	"testing"

	"github.com/twmb/franz-go/pkg/kfake"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

func TestScheduledKafkaProducesConsumesAndChecksBindings(t *testing.T) {
	cluster, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(1, "jobs"))
	if err != nil {
		t.Fatal(err)
	}
	defer cluster.Close()
	target := cluster.ListenAddrs()[0]
	endpoint, err := configureKafkaEndpoint(target, execution.KafkaOptions{SecurityProtocol: "PLAINTEXT"}, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	execute := scheduledKafkaExecutor(endpoint)
	request := project.Request{URL: "{{broker}}", Body: "AP8=", Variables: map[string]string{"broker": target, "topic": "jobs"}, BrokerTimeoutMS: 3000,
		Kafka: &project.KafkaSettings{Mode: "produce", Topic: "{{topic}}", Encoding: "base64"}}
	if result, err := execute(t.Context(), request); err != nil || !result.Success {
		t.Fatalf("produce: %+v %v", result, err)
	}
	request.Kafka = &project.KafkaSettings{Mode: "consume", Topic: "jobs", OffsetReset: "earliest", ReceiveLimit: "1", GroupID: "scheduled-fixture"}
	if result, err := execute(t.Context(), request); err != nil || !result.Success {
		t.Fatalf("consume: %+v %v", result, err)
	}
	request.Kafka = &project.KafkaSettings{Mode: "produce", Topic: "jobs"}
	for _, change := range []func(*project.Request){
		func(r *project.Request) { r.URL = "127.0.0.1:1" },
		func(r *project.Request) { r.Kafka.PasswordRef = "wrong" },
		func(r *project.Request) { r.Kafka.CAPEMRef = "wrong-ca" },
		func(r *project.Request) { r.BrokerTimeoutMS = 300001 },
	} {
		input := request
		settings := *request.Kafka
		input.Kafka = &settings
		change(&input)
		if result, err := execute(t.Context(), input); err == nil || result.Success {
			t.Fatal("invalid Kafka request accepted")
		}
	}
}
