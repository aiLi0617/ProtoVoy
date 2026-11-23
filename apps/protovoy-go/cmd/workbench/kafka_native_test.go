//go:build !js

package main

import (
	"testing"

	"github.com/zalando/go-keyring"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/secrets"
)

func TestDirectKafkaCanUseSystemKeychain(t *testing.T) {
	const target, ref = "localhost:9093", "jobs-password"
	keyring.MockInit()
	t.Setenv(execution.KafkaPasswordRefEnv, ref)
	t.Setenv(execution.KafkaPasswordEnv, "")
	t.Setenv("PROTOVOY_KAFKA_TARGET", target)
	t.Setenv("PROTOVOY_KAFKA_USERNAME", "reader")
	t.Setenv("PROTOVOY_KAFKA_SASL", "SCRAM-SHA-256")
	t.Setenv("PROTOVOY_KAFKA_CA_FILE", "")
	t.Setenv("PROTOVOY_KAFKA_PASSWORD_KEYCHAIN", "")
	options := execution.KafkaOptions{
		KafkaOperation: execution.KafkaOperation{Topic: "jobs"},
		PasswordRef:    ref, SecurityProtocol: "SASL_SSL",
	}
	if err := secrets.Put(ref, "fixture-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareDirectKafka(target, options); err == nil {
		t.Fatal("used the keychain without opt-in")
	}
	t.Setenv("PROTOVOY_KAFKA_PASSWORD_KEYCHAIN", "1")
	if _, err := prepareDirectKafka(target, options); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareDirectKafka("other.example:9093", options); err == nil {
		t.Fatal("accepted a different broker")
	}
}
