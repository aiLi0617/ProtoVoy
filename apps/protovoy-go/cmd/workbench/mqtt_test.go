package main

import (
	"bytes"
	"testing"

	"protovoy.dev/protovoy/internal/project"
)

func TestMQTTCollectionRoundTripKeepsSettingsWithoutPassword(t *testing.T) {
	w := newWorkbench()
	w.loadRequest(0, defaultMQTTRequest())
	w.target.SetText("mqtts://localhost")
	w.mqtt.topic.SetText("demo/#")
	w.mqtt.mode = "subscribe"
	w.mqtt.qos = 2
	w.mqtt.limit.SetText("3")
	w.mqtt.username.SetText("reader")
	w.mqtt.password.SetText("mqtt-fixture-password")
	w.saveDraft()
	if w.collection.selected != 1 {
		t.Fatalf("could not save MQTT request: %s", w.collection.message)
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
		if bytes.Contains(data, []byte(w.mqtt.password.Text())) {
			t.Fatal("MQTT password was persisted")
		}
	}
	value, err := project.Read(bytes.NewReader(portable))
	if err != nil {
		t.Fatal(err)
	}
	restored := newWorkbench()
	restored.loadRequest(1, value.Requests[0])
	if restored.draft() != w.draft() || restored.mqtt.password.Text() != "" {
		t.Fatal("MQTT settings did not round-trip or restored a password")
	}
}
