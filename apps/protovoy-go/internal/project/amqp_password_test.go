package project

import (
	"bytes"
	"strings"
	"testing"
)

func TestAMQPReferenceRoundTripsWithoutInlineCredentials(t *testing.T) {
	input := `{"protocolId":"amqp","target":"amqps://broker.example:5671/jobs","payload":{"type":"raw","value":{"routingKey":"jobs","passwordRef":"jobs"}}}`
	value, err := Read(strings.NewReader(input))
	if err != nil || len(value.Requests) != 1 || value.Requests[0].AMQP == nil || value.Requests[0].AMQP.PasswordRef != "jobs" {
		t.Fatalf("import lost reference: %v", err)
	}
	data, err := Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Read(bytes.NewReader(data))
	if err != nil || reopened.Requests[0].AMQP.PasswordRef != "jobs" {
		t.Fatalf("portable export lost reference: %v", err)
	}
	if _, err := Read(strings.NewReader(strings.Replace(input, `"passwordRef":"jobs"`, `"passwordRef":" jobs"`, 1))); err == nil {
		t.Fatal("accepted invalid reference")
	}
}
