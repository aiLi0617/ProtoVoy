package main

import (
	"github.com/zalando/go-keyring"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/secrets"
	"testing"
)

func TestAgentMQTTReferencesRequireExplicitKeychainAccessAndPinnedTarget(t *testing.T) {
	keyring.MockInit()
	t.Setenv(execution.MQTTPasswordRefEnv, "jobs")
	t.Setenv(execution.MQTTPasswordEnv, "")
	t.Setenv("PROTOVOY_MQTT_TARGET", "mqtts://broker.example:8883")
	t.Setenv("PROTOVOY_MQTT_USERNAME", "reader")
	t.Setenv("PROTOVOY_MQTT_SERVER_NAME", "")
	t.Setenv("PROTOVOY_MQTT_CA_FILE", "")
	if err := secrets.Put("jobs", "fixture-private-password"); err != nil {
		t.Fatal(err)
	}
	envelope := requestEnvelope{Target: "mqtts://broker.example:8883", Payload: []byte(`{"type":"raw","value":{"literal":true,"mode":"publish","topic":"jobs","passwordRef":"jobs"}}`)}
	if _, err := envelope.prepareMQTT(false); err == nil {
		t.Fatal("used keychain without opt-in")
	}
	if _, err := envelope.prepareMQTT(true); err != nil {
		t.Fatal(err)
	}
	envelope.Target = "mqtts://other.example:8883"
	if _, err := envelope.prepareMQTT(true); err == nil {
		t.Fatal("sent named credentials to another broker")
	}
}
