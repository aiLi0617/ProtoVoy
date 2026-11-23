//go:build !js

package execution

import (
	"errors"
	"strings"
	"testing"
)

func TestRedisReferenceChecksConnectionSettingsBeforeSecretLookup(t *testing.T) {
	const target = "broker.example:6380"
	t.Setenv(RedisPasswordRefEnv, "jobs")
	t.Setenv("PROTOVOY_REDIS_TARGET", target)
	t.Setenv("PROTOVOY_REDIS_USERNAME", "reader")
	t.Setenv("PROTOVOY_REDIS_DATABASE", "3")
	t.Setenv("PROTOVOY_REDIS_SERVER_NAME", "broker.example")
	t.Setenv("PROTOVOY_REDIS_CA_FILE", "")
	valid := RedisOptions{PasswordRef: "jobs", TLS: true, Protocol: 3}
	for name, mutate := range map[string]func(*RedisOptions){
		"plaintext":        func(o *RedisOptions) { o.TLS = false },
		"inline password":  func(o *RedisOptions) { o.Password = "override" },
		"other user":       func(o *RedisOptions) { o.Username = "other" },
		"other database":   func(o *RedisOptions) { o.Database = "4" },
		"invalid database": func(o *RedisOptions) { o.Database = "-1" },
		"CA override":      func(o *RedisOptions) { o.CAPEM = "override" },
		"name override":    func(o *RedisOptions) { o.ServerName = "other.example" },
		"other reference":  func(o *RedisOptions) { o.PasswordRef = "other" },
	} {
		t.Run(name, func(t *testing.T) {
			o := valid
			mutate(&o)
			if _, err := ResolveRedisOptions(target, o, func(string) (string, error) { t.Fatal("read secret for conflicting settings"); return "", nil }); err == nil {
				t.Fatal("accepted conflicting settings")
			}
		})
	}
	if _, err := ResolveRedisOptions("other.example:6380", valid, func(string) (string, error) { t.Fatal("read secret for another server"); return "", nil }); err == nil {
		t.Fatal("accepted different endpoint")
	}
	valid.Database = "03"
	resolved, err := ResolveRedisOptions(target, valid, func(ref string) (string, error) {
		if ref != "jobs" {
			t.Fatal("wrong reference")
		}
		return "fixture-private-password", nil
	})
	if err != nil || resolved.Password != "fixture-private-password" || resolved.Username != "reader" || resolved.Database != "3" || resolved.ServerName != "broker.example" || resolved.PasswordRef != "" || !resolved.TLS || resolved.Protocol != 3 {
		t.Fatalf("reference resolution failed: %v", err)
	}
	if _, err := ResolveRedisOptions(target, valid, func(string) (string, error) { return "", errors.New("fixture-private-password") }); err == nil || strings.Contains(err.Error(), "fixture-private-password") {
		t.Fatal("secret lookup leaked private details")
	}
}
