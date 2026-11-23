package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kfake"
	"github.com/zalando/go-keyring"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/secrets"
)

func TestAgentKafkaProducesConsumesAndResumesOverSASLTLS(t *testing.T) {
	const username, password = "reader", "kafka-fixture-password"
	tlsFixture := httptest.NewTLSServer(http.NotFoundHandler())
	tlsConfig := tlsFixture.TLS.Clone()
	tlsConfig.ClientAuth = tls.RequireAnyClientCert
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: tlsFixture.Certificate().Raw})
	key, err := x509.MarshalPKCS8PrivateKey(tlsConfig.Certificates[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	privateKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key})
	tlsFixture.Close()
	cluster, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(2, "jobs"),
		kfake.TLS(tlsConfig), kfake.EnableSASL(), kfake.Superuser("SCRAM-SHA-256", username, password))
	if err != nil {
		t.Fatal(err)
	}
	defer cluster.Close()
	caPath := filepath.Join(t.TempDir(), "kafka-ca.pem")
	if err := os.WriteFile(caPath, ca, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(execution.KafkaPasswordRefEnv, "jobs-password")
	t.Setenv(execution.KafkaPasswordEnv, password)
	t.Setenv("PROTOVOY_KAFKA_TARGET", cluster.ListenAddrs()[0])
	t.Setenv("PROTOVOY_KAFKA_USERNAME", username)
	t.Setenv("PROTOVOY_KAFKA_SASL", "SCRAM-SHA-256")
	t.Setenv("PROTOVOY_KAFKA_CA_FILE", caPath)
	t.Setenv("PROTOVOY_KAFKA_CA_REF", "jobs-ca")
	t.Setenv("PROTOVOY_KAFKA_CERT_REF", "jobs-client")
	t.Setenv("PROTOVOY_KAFKA_CERT_FILE", caPath)
	keyPath := filepath.Join(t.TempDir(), "kafka-key.pem")
	if err := os.WriteFile(keyPath, privateKey, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROTOVOY_KAFKA_KEY_REF", "jobs-key")
	t.Setenv("PROTOVOY_KAFKA_KEY_FILE", keyPath)
	agent := testAgent(t)
	client := &http.Client{Timeout: 12 * time.Second}
	call := func(method, path string, body []byte) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, agent.URL+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+pairingFixture)
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	run := func(value map[string]any) map[string]any {
		t.Helper()
		value["topic"], value["securityProtocol"] = "jobs", "SASL_SSL"
		value["passwordRef"], value["saslMechanism"] = "jobs-password", "SCRAM-SHA-256"
		value["caPemRef"], value["certificatePemRef"], value["keyPemRef"] = "jobs-ca", "jobs-client", "jobs-key"
		input, err := json.Marshal(map[string]any{
			"id": "480b7831-08f1-4753-aef3-887ba1324922", "protocolId": "kafka", "target": cluster.ListenAddrs()[0],
			"timeoutMs": 8_000, "payload": map[string]any{"type": "raw", "value": value},
		})
		if err != nil {
			t.Fatal(err)
		}
		started := call("POST", "/v1/executions", input)
		defer started.Body.Close()
		var response struct{ ExecutionID string }
		if started.StatusCode != http.StatusAccepted || json.NewDecoder(started.Body).Decode(&response) != nil {
			t.Fatalf("Kafka execution rejected: HTTP %d", started.StatusCode)
		}
		events := call("GET", "/v1/executions/"+response.ExecutionID+"/events", nil)
		defer events.Body.Close()
		reader := bufio.NewScanner(events.Body)
		var data []byte
		completed := false
		for reader.Scan() {
			event := readTestEvent(t, reader.Text())
			switch event["type"] {
			case "response_chunk":
				if chunk, ok := event["data_base64"].(string); ok {
					data, err = base64.StdEncoding.AppendDecode(data, []byte(chunk))
					if err != nil {
						t.Fatal(err)
					}
				}
			case "completed":
				completed = true
			case "failed":
				t.Fatalf("Kafka execution failed: %v", event["message"])
			}
		}
		var result map[string]any
		if !completed || reader.Err() != nil || json.Unmarshal(data, &result) != nil || bytes.Contains(data, []byte(password)) {
			t.Fatalf("unexpected Kafka execution result: completed=%v, error=%v", completed, reader.Err())
		}
		return result
	}
	for offset, payload := range []string{"AP8=", "aGVsbG8=", "dGhpcmQ="} {
		result := run(map[string]any{"mode": "produce", "partition": 1, "key": "fixture", "encoding": "base64", "payload": payload})
		if result["produced"] != true || result["partition"] != float64(1) || result["offset"] != float64(offset) {
			t.Fatalf("Kafka acknowledgement lost delivery metadata: %v", result)
		}
	}
	first := run(map[string]any{"mode": "consume", "groupId": "test-group", "offsetReset": "earliest"})
	if first["payload"] != nil || first["payloadBase64"] != "AP8=" || first["offset"] != float64(0) || first["key"] != "fixture" {
		t.Fatalf("unexpected first Kafka record: %v", first)
	}
	second := run(map[string]any{"mode": "consume", "groupId": "test-group", "offsetReset": "earliest", "autoCommit": true})
	if second["payload"] != "hello" || second["offset"] != float64(1) || second["partition"] != float64(1) {
		t.Fatalf("Kafka group did not resume from its committed offset: %v", second)
	}
	third := run(map[string]any{"mode": "consume", "groupId": "test-group", "offsetReset": "earliest"})
	if third["payload"] != "third" || third["offset"] != float64(2) {
		t.Fatalf("Kafka group did not resume after auto commit: %v", third)
	}
}

func TestAgentKafkaRejectsPlaintextCredentialsAndStoredReferences(t *testing.T) {
	for _, payload := range []string{
		`{"type":"raw","value":{"topic":"jobs","username":"reader","password":"fixture"}}`,
		`{"type":"raw","value":{"topic":"jobs","passwordRef":"fixture"}}`,
		`{"type":"raw","value":{"topic":"jobs","certificatePemRef":"fixture"}}`,
		`{"type":"raw","value":{"topic":"jobs","securityProtocol":"SSL","caPemRef":"fixture"}}`,
	} {
		envelope := requestEnvelope{Target: "localhost:9092", Payload: json.RawMessage(payload)}
		if _, err := envelope.prepareKafka(false); err == nil || strings.Contains(err.Error(), "fixture") {
			t.Fatal("expected a sanitized Kafka credential error")
		}
	}
}

func TestAgentKafkaUsesKeychainOnlyWhenEnabled(t *testing.T) {
	const target = "localhost:9093"
	keyring.MockInit()
	t.Setenv(execution.KafkaPasswordRefEnv, "jobs-password")
	t.Setenv(execution.KafkaPasswordEnv, "")
	t.Setenv("PROTOVOY_KAFKA_TARGET", target)
	t.Setenv("PROTOVOY_KAFKA_USERNAME", "reader")
	t.Setenv("PROTOVOY_KAFKA_SASL", "SCRAM-SHA-256")
	t.Setenv("PROTOVOY_KAFKA_CA_FILE", "")
	envelope := requestEnvelope{
		ID: "00000000-0000-4000-8000-000000000001", Protocol: "kafka",
		Target: "{{broker}}", Variables: map[string]string{"broker": target},
		Payload: json.RawMessage(`{"type":"raw","value":{"topic":"jobs","securityProtocol":"SASL_SSL","passwordRef":"jobs-password"}}`),
	}
	agent := &agentService{kafkaPasswordKeychain: true}
	if _, err := agent.prepare(envelope); err == nil {
		t.Fatal("accepted a missing keychain entry")
	}
	if err := secrets.Put("jobs-password", "fixture-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := (&agentService{}).prepare(envelope); err == nil {
		t.Fatal("used the keychain without opt-in")
	}
	if plan, err := agent.prepare(envelope); err != nil || plan.execute == nil {
		t.Fatalf("keychain-backed Kafka request was rejected: %v", err)
	}
	envelope.Variables["broker"] = "other.example:9093"
	if _, err := agent.prepare(envelope); err == nil {
		t.Fatal("accepted the credential for a different resolved broker")
	}
}
