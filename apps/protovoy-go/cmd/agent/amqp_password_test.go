package main

import (
	"github.com/zalando/go-keyring"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/secrets"
	"testing"
)

func TestAgentAMQPReferenceRequiresKeychainOptInAndPinnedVirtualHost(t *testing.T) {
	keyring.MockInit()
	t.Setenv(execution.AMQPPasswordRefEnv, "jobs")
	t.Setenv(execution.AMQPPasswordEnv, "")
	t.Setenv("PROTOVOY_AMQP_TARGET", "amqps://broker.example:5671/jobs")
	t.Setenv("PROTOVOY_AMQP_USERNAME", "reader")
	t.Setenv("PROTOVOY_AMQP_SERVER_NAME", "")
	t.Setenv("PROTOVOY_AMQP_CA_FILE", "")
	if err := secrets.Put("jobs", "fixture-private-password"); err != nil {
		t.Fatal(err)
	}
	envelope := requestEnvelope{Target: "amqps://broker.example:5671/jobs", Payload: []byte(`{"type":"raw","value":{"literal":true,"mode":"publish","routingKey":"jobs","passwordRef":"jobs"}}`)}
	if _, err := envelope.prepareAMQP(false); err == nil {
		t.Fatal("read keychain without opt-in")
	}
	if _, err := envelope.prepareAMQP(true); err != nil {
		t.Fatal(err)
	}
	envelope.Target = "amqps://broker.example:5671/other"
	if _, err := envelope.prepareAMQP(true); err == nil {
		t.Fatal("accepted another virtual host")
	}
}
