package main

import (
	"bytes"
	"testing"

	"protovoy.dev/protovoy/internal/project"
)

func TestAMQPCollectionRoundTripKeepsOptionsWithoutPassword(t *testing.T) {
	w := newWorkbench()
	w.loadRequest(0, defaultAMQPRequest())
	w.target.SetText("amqps://localhost/jobs")
	w.amqp.mode = "consume"
	w.amqp.exchange.SetText("events")
	w.amqp.exchangeType.SetText("topic")
	w.amqp.key.SetText("events.#")
	w.amqp.queue.SetText("jobs")
	w.amqp.limit.SetText("3")
	w.amqp.durable = true
	w.amqp.username.SetText("reader")
	w.amqp.password.SetText("amqp-fixture-password")
	w.saveDraft()
	if w.collection.selected != 1 {
		t.Fatalf("could not save AMQP request: %s", w.collection.message)
	}
	portable, err := project.Encode(*w.collection.project)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := project.EncodeWorkspace(project.Workspace{Draft: w.draft(), Project: w.collection.project, Selected: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{portable, snapshot} {
		if bytes.Contains(data, []byte(w.amqp.password.Text())) {
			t.Fatal("AMQP password was persisted")
		}
	}
	value, err := project.Read(bytes.NewReader(portable))
	if err != nil {
		t.Fatal(err)
	}
	restored := newWorkbench()
	restored.loadRequest(1, value.Requests[0])
	if restored.draft() != w.draft() || restored.amqp.password.Text() != "" {
		t.Fatal("AMQP options did not round-trip or restored a password")
	}
}
