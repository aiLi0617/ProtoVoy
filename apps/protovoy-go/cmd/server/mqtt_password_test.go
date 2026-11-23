package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHostedMQTTRejectsReferenceMismatchBeforeConnection(t *testing.T) {
	endpoint := &mqttEndpoint{target: "mqtts://broker.example:8883", username: "reader", password: "fixture-private-password", passwordRef: "jobs"}
	if err := validateMQTTPasswordReference(endpoint); err != nil {
		t.Fatal(err)
	}
	const origin = "http://127.0.0.1:39300"
	request := httptest.NewRequest(http.MethodPost, origin+"/api/mqtt", strings.NewReader(`{"mode":"publish","topic":"jobs","expectedPasswordRef":"other"}`))
	request.Header.Set("Origin", origin)
	request.Header.Set("X-ProtoVoy-Preview", "1")
	response := httptest.NewRecorder()
	mqttHandler(origin, endpoint).ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), endpoint.password) {
		t.Fatal("reference mismatch was not rejected safely")
	}
	endpoint.target = "mqtt://broker.example:1883"
	if err := validateMQTTPasswordReference(endpoint); err == nil {
		t.Fatal("accepted a hosted plaintext credential reference")
	}
}
