package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHostedAMQPRejectsMismatchedReferences(t *testing.T) {
	if err := validateAMQPPasswordReference("amqps://broker.example/jobs", "reader", "fixture-private-password", "jobs"); err != nil {
		t.Fatal(err)
	}
	if err := validateAMQPPasswordReference("amqp://broker.example/jobs", "reader", "fixture-private-password", "jobs"); err == nil {
		t.Fatal("accepted plaintext reference binding")
	}
	const origin = "http://127.0.0.1:39300"
	request := httptest.NewRequest(http.MethodPost, origin+"/api/amqp", strings.NewReader(`{"mode":"publish","routingKey":"jobs","expectedPasswordRef":"other"}`))
	request.Header.Set("Origin", origin)
	request.Header.Set("X-ProtoVoy-Preview", "1")
	response := httptest.NewRecorder()
	amqpHandler(origin, &amqpEndpoint{target: "amqps://broker.example/jobs", passwordRef: "jobs"}).ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "password reference does not match") {
		t.Fatal("mismatched reference reached connection preparation")
	}
}
