package main

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kfake"
	"protovoy.dev/protovoy/internal/execution"
)

func TestHostedKafkaRejectsInvalidTrustBeforePasswordLookup(t *testing.T) {
	ca := filepath.Join(t.TempDir(), "bad-ca.pem")
	if err := os.WriteFile(ca, []byte("invalid CA fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	options := execution.KafkaOptions{SecurityProtocol: "SASL_SSL", Username: "operator", PasswordRef: "jobs"}
	if _, err := configureKafkaEndpoint("localhost:9093", options, ca, "", "", func(string) (string, error) {
		t.Fatal("read a password before rejecting invalid TLS trust")
		return "", nil
	}); err == nil {
		t.Fatal("accepted invalid TLS trust")
	}
}

func TestKafkaBridgeRequiresAuthorizedSameOriginAndFixedBrokers(t *testing.T) {
	const origin = "https://workbench.example.com"
	access, err := configureWebAccess("127.0.0.1:39300", origin, "test-cert", "test-key", "operator", fixtureWebPassword)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := configureKafkaEndpoint("kafka.example.com:9093", execution.KafkaOptions{}, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	api := handler(access, t.TempDir(), "", "", socketTargets{Kafka: endpoint})
	for _, tc := range []struct {
		name, source, marker, body string
		auth                       bool
		want                       int
	}{
		{"anonymous", origin, "1", "{}", false, http.StatusUnauthorized},
		{"cross-origin", "https://other.example.com", "1", "{}", true, http.StatusForbidden},
		{"missing marker", origin, "", "{}", true, http.StatusForbidden},
		{"different target", origin, "1", `{"expectedTarget":"other.example.com:9093","topic":"jobs"}`, true, http.StatusConflict},
		{"credentials", origin, "1", `{"topic":"jobs","password":"other"}`, true, http.StatusBadRequest},
		{"unknown reference", origin, "1", `{"topic":"jobs","expectedPasswordRef":"other"}`, true, http.StatusBadRequest},
		{"unknown TLS reference", origin, "1", `{"topic":"jobs","expectedTlsReferences":{"caPemRef":"other"}}`, true, http.StatusBadRequest},
		{"security override", origin, "1", `{"topic":"jobs","securityProtocol":"PLAINTEXT"}`, true, http.StatusBadRequest},
		{"CA override", origin, "1", `{"topic":"jobs","caPem":"other"}`, true, http.StatusBadRequest},
		{"bad topic", origin, "1", `{"topic":"bad/topic"}`, true, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, origin+execution.KafkaBridgePath, strings.NewReader(tc.body))
			r.Header.Set("Origin", tc.source)
			r.Header.Set("X-ProtoVoy-Preview", tc.marker)
			if tc.auth {
				r.SetBasicAuth("operator", fixtureWebPassword)
			}
			w := httptest.NewRecorder()
			api.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("got %d, want %d", w.Code, tc.want)
			}
		})
	}
}

func TestKafkaBridgeProducesAndConsumesWithServerTLSCredentials(t *testing.T) {
	const password = "kafka-test-password"
	fixture := httptest.NewTLSServer(http.NotFoundHandler())
	config := fixture.TLS.Clone()
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: fixture.Certificate().Raw})
	fixture.Close()
	cluster, err := kfake.NewCluster(kfake.NumBrokers(1), kfake.SeedTopics(1, "jobs"),
		kfake.TLS(config), kfake.EnableSASL(), kfake.Superuser("SCRAM-SHA-256", "operator", password))
	if err != nil {
		t.Fatal(err)
	}
	defer cluster.Close()
	caFile := filepath.Join(t.TempDir(), "fixture-ca.pem")
	if err := os.WriteFile(caFile, ca, 0600); err != nil {
		t.Fatal(err)
	}
	endpoint, err := configureKafkaEndpoint(cluster.ListenAddrs()[0], execution.KafkaOptions{
		SecurityProtocol: "SASL_SSL", SASLMechanism: "SCRAM-SHA-256", Username: "operator", PasswordRef: "jobs",
		KafkaTLSReferences: execution.KafkaTLSReferences{CAPEMRef: "jobs-ca"},
	}, caFile, "", "", func(reference string) (string, error) {
		if reference != "jobs" {
			t.Fatalf("unexpected password reference %q", reference)
		}
		return password, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	const addr = "127.0.0.1:39300"
	api := handler(loopbackAccess(addr), t.TempDir(), "", "", socketTargets{Kafka: endpoint})
	for _, tc := range []struct{ body, result string }{
		{`{"mode":"produce","topic":"jobs","payload":"AP8=","encoding":"base64","expectedPasswordRef":"jobs","expectedTlsReferences":{"caPemRef":"jobs-ca"}}`, `"produced":true`},
		{`{"mode":"consume","topic":"jobs","offsetReset":"earliest","groupId":"fixture"}`, `"payloadBase64":"AP8="`},
	} {
		r := httptest.NewRequest(http.MethodPost, "http://"+addr+execution.KafkaBridgePath, strings.NewReader(tc.body))
		r.Header.Set("Origin", "http://"+addr)
		r.Header.Set("X-ProtoVoy-Preview", "1")
		w := httptest.NewRecorder()
		api.ServeHTTP(w, r)
		if w.Code != http.StatusOK || strings.Contains(w.Body.String(), password) || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("unexpected Kafka bridge status or headers: %d %v", w.Code, w.Header())
		}
		decoder := json.NewDecoder(w.Body)
		for _, eventType := range []string{"connected", "message", "completed"} {
			var event execution.BrokerBridgeEvent
			if err := decoder.Decode(&event); err != nil || event.Type != eventType {
				t.Fatalf("expected %s, got %+v (%v)", eventType, event, err)
			}
			if eventType == "message" && !strings.Contains(string(event.Message), tc.result) {
				t.Fatalf("unexpected Kafka result: %s", event.Message)
			}
		}
	}
}

type countingKafkaListener struct {
	net.Listener
	accepted atomic.Int32
}

func (listener *countingKafkaListener) Accept() (net.Conn, error) {
	conn, err := listener.Listener.Accept()
	if err == nil {
		listener.accepted.Add(1)
	}
	return conn, err
}

func TestHostedKafkaDoesNotDialUnlistedAdvertisedLeader(t *testing.T) {
	var listeners []*countingKafkaListener
	cluster, err := kfake.NewCluster(kfake.NumBrokers(2), kfake.SeedTopics(1, "jobs"),
		kfake.ListenFn(func(network, addr string) (net.Listener, error) {
			listener, err := net.Listen(network, addr)
			if err != nil {
				return nil, err
			}
			counted := &countingKafkaListener{Listener: listener}
			listeners = append(listeners, counted)
			return counted, nil
		}))
	if err != nil {
		t.Fatal(err)
	}
	defer cluster.Close()
	if err := cluster.MoveTopicPartition("jobs", 0, 1); err != nil {
		t.Fatal(err)
	}
	addresses := cluster.ListenAddrs()
	execute := func(target string) error {
		t.Helper()
		broker, err := execution.NewHostedKafka(target, execution.KafkaOptions{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		request, err := broker.Prepare(execution.KafkaOperation{Topic: "jobs", Payload: "fixture"})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		return request.Execute(ctx, func() error { return nil }, func(any) error { return nil })
	}
	if err := execute(addresses[0]); err == nil || listeners[0].accepted.Load() == 0 || listeners[1].accepted.Load() != 0 {
		t.Fatalf("unexpected broker dial result: %v, bootstrap=%d, leader=%d",
			err, listeners[0].accepted.Load(), listeners[1].accepted.Load())
	}
	if err := execute(strings.Join(addresses, ",")); err != nil || listeners[1].accepted.Load() == 0 {
		t.Fatalf("configured leader could not be reached: %v", err)
	}
}
