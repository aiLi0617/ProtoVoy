//go:build !js

package main

import (
	"github.com/zalando/go-keyring"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
	"protovoy.dev/protovoy/internal/secrets"
	"strings"
	"testing"
)

func TestRedisKeychainOptInAndSavedReference(t *testing.T) {
	keyring.MockInit()
	t.Setenv(execution.RedisPasswordRefEnv, "jobs")
	t.Setenv(execution.RedisPasswordEnv, "")
	t.Setenv("PROTOVOY_REDIS_TARGET", "broker.example:6380")
	t.Setenv("PROTOVOY_REDIS_USERNAME", "reader")
	t.Setenv("PROTOVOY_REDIS_DATABASE", "3")
	t.Setenv("PROTOVOY_REDIS_SERVER_NAME", "")
	t.Setenv("PROTOVOY_REDIS_CA_FILE", "")
	t.Setenv("PROTOVOY_REDIS_PASSWORD_KEYCHAIN", "")
	if err := secrets.Put("jobs", "fixture-private-password"); err != nil {
		t.Fatal(err)
	}
	options := execution.RedisOptions{PasswordRef: "jobs", TLS: true}
	if _, err := prepareDirectRedisOptions("broker.example:6380", options); err == nil {
		t.Fatal("read keychain without opt-in")
	}
	t.Setenv("PROTOVOY_REDIS_PASSWORD_KEYCHAIN", "1")
	if _, err := prepareDirectRedisOptions("broker.example:6380", options); err != nil {
		t.Fatal(err)
	}
	w := newWorkbench()
	w.loadRequest(0, project.Request{URL: "broker.example:6380", Body: `[["PING"]]`, Redis: &project.RedisSettings{Protocol: 2, TLS: true, PasswordRef: "jobs", Database: "3"}})
	w.redis.password.SetText("fixture-private-password")
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
	if restored.redis.passwordRef.Text() != "jobs" || restored.redis.password.Text() != "" {
		t.Fatal("workspace did not restore reference safely")
	}
}
