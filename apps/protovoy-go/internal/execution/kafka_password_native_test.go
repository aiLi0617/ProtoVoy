//go:build !js

package execution

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestKafkaPasswordReferenceRequiresMatchingTrustedSettings(t *testing.T) {
	const target = "localhost:9093"
	t.Setenv(KafkaPasswordRefEnv, "jobs")
	t.Setenv(KafkaPasswordEnv, "fixture-password")
	t.Setenv("PROTOVOY_KAFKA_TARGET", target)
	t.Setenv("PROTOVOY_KAFKA_USERNAME", "reader")
	t.Setenv("PROTOVOY_KAFKA_SASL", "SCRAM-SHA-256")
	t.Setenv("PROTOVOY_KAFKA_CA_FILE", "")
	valid := KafkaOptions{KafkaOperation: KafkaOperation{Topic: "jobs"}, PasswordRef: "jobs", SecurityProtocol: "SASL_SSL"}
	for name, mutate := range map[string]func(*KafkaOptions){
		"inline password":    func(o *KafkaOptions) { o.Password = "override" },
		"trust override":     func(o *KafkaOptions) { o.CAPEM = "untrusted certificate" },
		"username override":  func(o *KafkaOptions) { o.Username = "other" },
		"mechanism override": func(o *KafkaOptions) { o.SASLMechanism = "PLAIN" },
		"plaintext":          func(o *KafkaOptions) { o.SecurityProtocol = "PLAINTEXT" },
	} {
		t.Run(name, func(t *testing.T) {
			options := valid
			mutate(&options)
			if _, err := PrepareKafka(target, options); err == nil {
				t.Fatal("accepted conflicting connection settings")
			}
		})
	}
	if _, err := PrepareKafka("other.example:9093", valid); err == nil {
		t.Fatal("accepted a different Kafka target")
	}
	if _, err := PrepareKafka(target, valid); err != nil {
		t.Fatal(err)
	}
}

func TestKafkaPasswordResolverChecksBrokerBeforeReading(t *testing.T) {
	const target = "localhost:9093"
	t.Setenv(KafkaPasswordRefEnv, "jobs")
	t.Setenv("PROTOVOY_KAFKA_TARGET", target)
	t.Setenv("PROTOVOY_KAFKA_USERNAME", "reader")
	t.Setenv("PROTOVOY_KAFKA_SASL", "SCRAM-SHA-256")
	t.Setenv("PROTOVOY_KAFKA_CA_FILE", "")
	options := KafkaOptions{KafkaOperation: KafkaOperation{Topic: "jobs"}, PasswordRef: "jobs", SecurityProtocol: "SASL_SSL"}
	if _, err := PrepareKafkaWithPasswordResolver("other.example:9093", options, func(string) (string, error) {
		t.Fatal("read a credential for a different Kafka broker")
		return "", nil
	}); err == nil {
		t.Fatal("accepted a different Kafka broker")
	}
	if _, err := PrepareKafkaWithPasswordResolver(target, options, func(ref string) (string, error) {
		if ref != "jobs" {
			t.Fatalf("unexpected password reference %q", ref)
		}
		return "fixture-password", nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareKafkaWithPasswordResolver(target, options, func(string) (string, error) {
		return "", errors.New("private backend details")
	}); err == nil || err.Error() != "Kafka password reference could not be resolved" {
		t.Fatalf("unexpected error: %v", err)
	}
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, []byte("invalid CA fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROTOVOY_KAFKA_CA_FILE", ca)
	if _, err := PrepareKafkaWithPasswordResolver(target, options, func(string) (string, error) {
		t.Fatal("read a credential with an invalid Kafka CA")
		return "", nil
	}); err == nil {
		t.Fatal("accepted an invalid Kafka CA")
	}
}
