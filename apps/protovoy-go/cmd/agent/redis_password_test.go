package main

import (
	"github.com/zalando/go-keyring"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/secrets"
	"testing"
)

func TestAgentRedisReferenceNeedsOptInAndMatchingDatabase(t *testing.T) {
	keyring.MockInit()
	t.Setenv(execution.RedisPasswordRefEnv, "jobs")
	t.Setenv(execution.RedisPasswordEnv, "")
	t.Setenv("PROTOVOY_REDIS_TARGET", "broker.example:6380")
	t.Setenv("PROTOVOY_REDIS_USERNAME", "reader")
	t.Setenv("PROTOVOY_REDIS_DATABASE", "3")
	t.Setenv("PROTOVOY_REDIS_SERVER_NAME", "")
	t.Setenv("PROTOVOY_REDIS_CA_FILE", "")
	if err := secrets.Put("jobs", "fixture-private-password"); err != nil {
		t.Fatal(err)
	}
	envelope := requestEnvelope{Target: "rediss://broker.example:6380", Payload: []byte(`{"type":"raw","value":{"literal":true,"commands":[["PING"]],"passwordRef":"jobs","database":"3"}}`)}
	if _, _, err := envelope.prepareRedis(false); err == nil {
		t.Fatal("used keychain without opt-in")
	}
	if _, _, err := envelope.prepareRedis(true); err != nil {
		t.Fatal(err)
	}
	envelope.Payload = []byte(`{"type":"raw","value":{"literal":true,"commands":[["PING"]],"passwordRef":"jobs","database":"4"}}`)
	if _, _, err := envelope.prepareRedis(true); err == nil {
		t.Fatal("accepted another initial database")
	}
}
