package project

import (
	"bytes"
	"strings"
	"testing"
)

func TestRedisReferenceSurvivesPortableProject(t *testing.T) {
	input := `{"protocolId":"redis","target":"broker.example:6380","payload":{"type":"raw","value":{"commands":[["PING"]],"tls":true,"database":3,"passwordRef":"jobs"}}}`
	value, err := Read(strings.NewReader(input))
	if err != nil || len(value.Requests) != 1 || value.Requests[0].Redis == nil || value.Requests[0].Redis.PasswordRef != "jobs" {
		t.Fatalf("import lost reference: %v", err)
	}
	data, err := Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Read(bytes.NewReader(data))
	if err != nil || reopened.Requests[0].Redis.PasswordRef != "jobs" || reopened.Requests[0].Redis.Database != "3" {
		t.Fatalf("portable project lost credential settings: %v", err)
	}
	if _, err := Read(strings.NewReader(strings.Replace(input, `"passwordRef":"jobs"`, `"passwordRef":" jobs"`, 1))); err == nil {
		t.Fatal("accepted an invalid reference name")
	}
}
