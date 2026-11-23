//go:build !js

package main

import (
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
	"protovoy.dev/protovoy/internal/secrets"
)

func TestMQTTSystemCredentialOptInAndWorkspaceReference(t *testing.T) {
	keyring.MockInit()
	const target = "mqtts://broker.example:8883"
	t.Setenv(execution.MQTTPasswordRefEnv, "jobs")
	t.Setenv(execution.MQTTPasswordEnv, "")
	t.Setenv("PROTOVOY_MQTT_TARGET", target)
	t.Setenv("PROTOVOY_MQTT_USERNAME", "reader")
	t.Setenv("PROTOVOY_MQTT_SERVER_NAME", "")
	t.Setenv("PROTOVOY_MQTT_CA_FILE", "")
	t.Setenv("PROTOVOY_MQTT_PASSWORD_KEYCHAIN", "")
	if err := secrets.Put("jobs", "fixture-private-password"); err != nil {
		t.Fatal(err)
	}
	options := execution.MQTTOptions{PasswordRef: "jobs", Mode: "publish", Topic: "jobs", ReceiveLimit: 1}
	if _, err := prepareDirectMQTT(target, options); err == nil {
		t.Fatal("read keychain without opt-in")
	}
	t.Setenv("PROTOVOY_MQTT_PASSWORD_KEYCHAIN", "1")
	if _, err := prepareDirectMQTT(target, options); err != nil {
		t.Fatal(err)
	}
	w := newWorkbench()
	w.loadRequest(0, project.Request{URL: target, MQTT: &project.MQTTSettings{Mode: "publish", Topic: "jobs", PasswordRef: "jobs", ReceiveLimit: 1}})
	w.mqtt.password.SetText("fixture-private-password")
	data, err := project.EncodeWorkspace(w.currentWorkspace())
	if err != nil || strings.Contains(string(data), "fixture-private-password") || !strings.Contains(string(data), `"passwordRef":"jobs"`) {
		t.Fatalf("workspace lost reference or retained password: %v", err)
	}
	workspace, err := project.DecodeWorkspace(data)
	if err != nil {
		t.Fatal(err)
	}
	restored := newWorkbench()
	restored.applyWorkspace(workspace)
	if restored.mqtt.passwordRef.Text() != "jobs" || restored.mqtt.password.Text() != "" {
		t.Fatal("restored inline password or lost reference")
	}
}
