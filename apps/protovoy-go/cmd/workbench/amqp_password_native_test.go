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

func TestAMQPKeychainOptInAndSavedReference(t *testing.T) {
	keyring.MockInit()
	const target = "amqps://broker.example:5671/jobs"
	t.Setenv(execution.AMQPPasswordRefEnv, "jobs")
	t.Setenv(execution.AMQPPasswordEnv, "")
	t.Setenv("PROTOVOY_AMQP_TARGET", target)
	t.Setenv("PROTOVOY_AMQP_USERNAME", "reader")
	t.Setenv("PROTOVOY_AMQP_SERVER_NAME", "")
	t.Setenv("PROTOVOY_AMQP_CA_FILE", "")
	t.Setenv("PROTOVOY_AMQP_PASSWORD_KEYCHAIN", "")
	if err := secrets.Put("jobs", "fixture-private-password"); err != nil {
		t.Fatal(err)
	}
	options := execution.AMQPOptions{PasswordRef: "jobs", Mode: "publish", ExchangeType: "direct", RoutingKey: "jobs", ReceiveLimit: 1}
	if _, err := prepareDirectAMQP(target, options); err == nil {
		t.Fatal("used keychain without opt-in")
	}
	t.Setenv("PROTOVOY_AMQP_PASSWORD_KEYCHAIN", "1")
	if _, err := prepareDirectAMQP(target, options); err != nil {
		t.Fatal(err)
	}
	w := newWorkbench()
	w.loadRequest(0, project.Request{URL: target, AMQP: &project.AMQPSettings{Mode: "publish", ExchangeType: "direct", RoutingKey: "jobs", ReceiveLimit: 1, PasswordRef: "jobs"}})
	w.amqp.password.SetText("fixture-private-password")
	data, err := project.EncodeWorkspace(w.currentWorkspace())
	if err != nil || strings.Contains(string(data), "fixture-private-password") || !strings.Contains(string(data), `"passwordRef":"jobs"`) {
		t.Fatalf("workspace leaked password or lost reference: %v", err)
	}
	workspace, err := project.DecodeWorkspace(data)
	if err != nil {
		t.Fatal(err)
	}
	restored := newWorkbench()
	restored.applyWorkspace(workspace)
	if restored.amqp.passwordRef.Text() != "jobs" || restored.amqp.password.Text() != "" {
		t.Fatal("workspace did not restore the reference safely")
	}
}
