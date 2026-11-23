package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/secrets"
)

func TestSQLExecutionEnvelope(t *testing.T) {
	agent := &agentService{}
	envelope := requestEnvelope{
		ID: "00000000-0000-4000-8000-000000000001", Protocol: "sql",
		Target:  "postgres://localhost/database",
		Payload: json.RawMessage(`{"type":"raw","value":{"sql":"SELECT $1","parameters":[42],"transactional":true}}`),
	}
	if plan, err := agent.prepare(envelope); err != nil || plan.execute == nil {
		t.Fatalf("SQL envelope was not accepted: %v", err)
	}
	envelope.Payload = json.RawMessage(`{"type":"raw","value":{"sql":"SELECT 1","passwordRef":"private-reference"}}`)
	if _, err := agent.prepare(envelope); err == nil || strings.Contains(err.Error(), "private-reference") {
		t.Fatal("expected a sanitized error for an unsupported password reference")
	}
}

func TestAgentSQLiteFilesRequireExactOptInAfterVariableResolution(t *testing.T) {
	const allowed = "sqlite:fixture.db?mode=ro"
	envelope := requestEnvelope{
		ID: "00000000-0000-4000-8000-000000000001", Protocol: "sql",
		Target: "{{database}}", Variables: map[string]string{"database": allowed},
		Payload: json.RawMessage(`{"type":"raw","value":{"sql":"SELECT 1"}}`),
	}
	if _, err := (&agentService{}).prepare(envelope); err == nil {
		t.Fatal("Agent accepted a file target without explicit opt-in")
	}
	agent := &agentService{sqliteTarget: allowed}
	if _, err := agent.prepare(envelope); err != nil {
		t.Fatal(err)
	}
	envelope.Variables["database"] = "sqlite:fixture.db?mode=rwc"
	if _, err := agent.prepare(envelope); err == nil {
		t.Fatal("Agent allowed a different SQLite access mode")
	}
	envelope.Variables["database"] = "sqlite::memory:"
	if _, err := (&agentService{}).prepare(envelope); err != nil {
		t.Fatal(err)
	}
}

func TestAgentBindsSQLPasswordReferenceToResolvedTarget(t *testing.T) {
	const target = "postgres://database.example/jobs"
	keyring.MockInit()
	t.Setenv(execution.SQLPasswordRefEnv, "jobs")
	t.Setenv(execution.SQLTargetEnv, target)
	t.Setenv(execution.SQLUsernameEnv, "reader")
	t.Setenv(execution.SQLPasswordEnv, "")
	if _, err := loadAgentSQLPasswordBinding(true); err == nil {
		t.Fatal("missing keychain entry did not stop startup")
	}
	if err := secrets.Put("jobs", "fixture-password"); err != nil {
		t.Fatal(err)
	}
	binding, err := loadAgentSQLPasswordBinding(true)
	if err != nil {
		t.Fatal(err)
	}
	agent := &agentService{sqlPasswords: binding}
	envelope := requestEnvelope{
		ID: "00000000-0000-4000-8000-000000000001", Protocol: "sql",
		Target: "{{database}}", Variables: map[string]string{"database": target},
		Payload: json.RawMessage(`{"type":"raw","value":{"sql":"SELECT 1","passwordRef":"jobs"}}`),
	}
	if _, err := agent.prepare(envelope); err != nil {
		t.Fatal(err)
	}
	envelope.Variables["database"] = "postgres://other.example/jobs"
	if _, err := agent.prepare(envelope); err == nil || strings.Contains(err.Error(), "fixture-password") {
		t.Fatal("Agent did not reject a password reference at another target")
	}
	envelope.Variables["database"] = target
	envelope.Payload = json.RawMessage(`{"type":"raw","value":{"sql":"SELECT 1","passwordRef":"jobs","username":"other"}}`)
	if _, err := agent.prepare(envelope); err == nil {
		t.Fatal("Agent accepted a different username")
	}
}
