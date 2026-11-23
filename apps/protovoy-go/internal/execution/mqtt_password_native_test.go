//go:build !js

package execution

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestMQTTReferenceChecksBindingBeforeReadingSecret(t *testing.T) {
	const target = "mqtts://broker.example:8883"
	t.Setenv(MQTTPasswordRefEnv, "jobs")
	t.Setenv("PROTOVOY_MQTT_TARGET", target)
	t.Setenv("PROTOVOY_MQTT_USERNAME", "reader")
	t.Setenv("PROTOVOY_MQTT_SERVER_NAME", "broker.example")
	t.Setenv("PROTOVOY_MQTT_CA_FILE", "")
	valid := MQTTOptions{PasswordRef: "jobs", Mode: "publish", Topic: "jobs", ReceiveLimit: 1, KeepAlive: 30, CleanSession: true}
	for name, mutate := range map[string]func(*MQTTOptions){
		"inline password":   func(o *MQTTOptions) { o.Password = "override" },
		"other user":        func(o *MQTTOptions) { o.Username = "other" },
		"CA override":       func(o *MQTTOptions) { o.CAPEM = "override" },
		"TLS name override": func(o *MQTTOptions) { o.ServerName = "other.example" },
		"other ref":         func(o *MQTTOptions) { o.PasswordRef = "other" },
		"template ref":      func(o *MQTTOptions) { o.PasswordRef = "{{name}}" },
	} {
		t.Run(name, func(t *testing.T) {
			options := valid
			mutate(&options)
			if _, err := PrepareMQTTWithPasswordResolver(target, options, func(string) (string, error) { t.Fatal("read secret for an invalid binding"); return "", nil }); err == nil {
				t.Fatal("accepted conflicting MQTT settings")
			}
		})
	}
	for _, badTarget := range []string{"mqtts://other.example:8883", "mqtt://broker.example:1883"} {
		if _, err := PrepareMQTTWithPasswordResolver(badTarget, valid, func(string) (string, error) { t.Fatal("read secret for another endpoint"); return "", nil }); err == nil {
			t.Fatal("accepted different target")
		}
	}
	request, err := PrepareMQTTWithPasswordResolver(target, valid, func(ref string) (string, error) {
		if ref != "jobs" {
			t.Fatal("wrong reference")
		}
		return "fixture-private-password", nil
	})
	if err != nil || request.tls == nil || request.tls.InsecureSkipVerify || request.tls.ServerName != "broker.example" || !bytes.Contains(request.connect, []byte("fixture-private-password")) {
		t.Fatalf("reference preparation failed: %v", err)
	}
	if valid.Password != "" {
		t.Fatal("modified caller's credential options")
	}
	if _, err := PrepareMQTTWithPasswordResolver(target, valid, func(string) (string, error) { return "", errors.New("fixture-private-password") }); err == nil || strings.Contains(err.Error(), "fixture-private-password") {
		t.Fatal("resolver failure leaked secret details")
	}
	t.Setenv("PROTOVOY_MQTT_TARGET", "mqtt://broker.example:1883")
	if _, err := PrepareMQTTWithPasswordResolver("mqtt://broker.example:1883", valid, func(string) (string, error) { t.Fatal("read secret for plaintext endpoint"); return "", nil }); err == nil {
		t.Fatal("accepted a configured plaintext endpoint")
	}
}
