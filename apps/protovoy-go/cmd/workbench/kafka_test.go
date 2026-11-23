package main

import (
	"bytes"
	"testing"

	"github.com/vivaui/viva/text"
	"protovoy.dev/protovoy/internal/project"
)

func TestKafkaCollectionRoundTripOmitsSessionCredentials(t *testing.T) {
	w := newWorkbench()
	w.loadRequest(0, defaultKafkaRequest())
	w.target.SetText("localhost:9093")
	w.kafka.topic.SetText("jobs")
	w.kafka.partition.SetText("0")
	w.kafka.security.SetText("SASL_SSL")
	w.kafka.username.SetText("reader")
	w.kafka.passwordRef.SetText("jobs-password")
	w.kafka.caRef.SetText("jobs-ca")
	w.kafka.certRef.SetText("jobs-cert")
	w.kafka.keyRef.SetText("jobs-key")
	w.kafka.password.SetText("fixture-session-password")
	w.kafka.ca.input.Editor = text.NewEditor("fixture-session-ca")
	w.kafka.finishTLSImport(kafkaTLSImport{data: []byte("fixture-session-certificate")})
	w.kafka.finishTLSImport(kafkaTLSImport{key: true, data: []byte("fixture-session-private-key")})
	w.requestBody.input.Editor = text.NewEditor("fixture message")
	w.saveDraft()
	if w.collection.selected != 1 {
		t.Fatalf("Kafka request was not saved: %s", w.collection.message)
	}
	if w.kafka.certificate == "" || w.kafka.privateKey == "" {
		t.Fatal("saving cleared the current Kafka client identity")
	}
	portable, err := project.Encode(*w.collection.project)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := project.EncodeWorkspace(project.Workspace{Draft: w.draft(), Project: w.collection.project, Selected: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{portable, workspace} {
		if bytes.Contains(data, []byte("fixture-session-")) || !bytes.Contains(data, []byte("jobs-password")) {
			t.Fatal("Kafka session material was persisted")
		}
	}
	restored, err := project.Read(bytes.NewReader(portable))
	if err != nil {
		t.Fatal(err)
	}
	other := newWorkbench()
	other.loadRequest(1, restored.Requests[0])
	if other.draft() != w.draft() || other.kafka.password.Text() != "" || other.kafka.certificate != "" || other.kafka.privateKey != "" {
		t.Fatal("Kafka request did not round-trip")
	}
}
