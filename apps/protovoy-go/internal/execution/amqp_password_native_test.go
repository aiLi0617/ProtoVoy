//go:build !js

package execution

import (
	"errors"
	"strings"
	"testing"
)

func TestAMQPReferencePinsEndpointVirtualHostAndTrustBeforeReading(t *testing.T) {
	const target = "amqps://broker.example:5671/jobs"
	t.Setenv(AMQPPasswordRefEnv, "jobs")
	t.Setenv("PROTOVOY_AMQP_TARGET", target)
	t.Setenv("PROTOVOY_AMQP_USERNAME", "reader")
	t.Setenv("PROTOVOY_AMQP_SERVER_NAME", "broker.example")
	t.Setenv("PROTOVOY_AMQP_CA_FILE", "")
	valid := AMQPOptions{PasswordRef: "jobs", Mode: "publish", ExchangeType: "direct", RoutingKey: "jobs", ReceiveLimit: 1}
	for name, mutate := range map[string]func(*AMQPOptions){
		"inline password":    func(o *AMQPOptions) { o.Password = "override" },
		"username override":  func(o *AMQPOptions) { o.Username = "other" },
		"CA override":        func(o *AMQPOptions) { o.CAPEM = "override" },
		"TLS name override":  func(o *AMQPOptions) { o.ServerName = "other.example" },
		"reference mismatch": func(o *AMQPOptions) { o.PasswordRef = "other" },
	} {
		t.Run(name, func(t *testing.T) {
			options := valid
			mutate(&options)
			if _, err := PrepareAMQPWithPasswordResolver(target, options, func(string) (string, error) { t.Fatal("read secret before checking binding"); return "", nil }); err == nil {
				t.Fatal("accepted unsafe connection settings")
			}
		})
	}
	for _, target := range []string{"amqps://other.example:5671/jobs", "amqps://broker.example:5671/other", "amqp://broker.example:5672/jobs"} {
		if _, err := PrepareAMQPWithPasswordResolver(target, valid, func(string) (string, error) {
			t.Fatal("read secret for another endpoint or virtual host")
			return "", nil
		}); err == nil {
			t.Fatal("accepted another endpoint")
		}
	}
	request, err := PrepareAMQPWithPasswordResolver(target, valid, func(ref string) (string, error) {
		if ref != "jobs" {
			t.Fatal("wrong reference")
		}
		return "fixture-private-password", nil
	})
	if err != nil || request.broker.tls == nil || request.broker.tls.InsecureSkipVerify || request.broker.tls.ServerName != "broker.example" || request.broker.uri.Vhost != "jobs" || request.broker.uri.Username != "reader" || request.broker.uri.Password != "fixture-private-password" || request.options.Password != "" {
		t.Fatalf("reference preparation failed: %v", err)
	}
	if _, err := PrepareAMQPWithPasswordResolver(target, valid, func(string) (string, error) { return "", errors.New("fixture-private-password") }); err == nil || strings.Contains(err.Error(), "fixture-private-password") {
		t.Fatal("resolver failure leaked secret details")
	}
	t.Setenv("PROTOVOY_AMQP_TARGET", "amqp://broker.example:5672/jobs")
	if _, err := PrepareAMQPWithPasswordResolver("amqp://broker.example:5672/jobs", valid, func(string) (string, error) { t.Fatal("read secret for plaintext binding"); return "", nil }); err == nil {
		t.Fatal("accepted configured plaintext binding")
	}
}
