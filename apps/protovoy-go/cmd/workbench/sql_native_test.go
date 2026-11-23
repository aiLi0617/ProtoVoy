//go:build !js

package main

import (
	"testing"

	"github.com/zalando/go-keyring"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/secrets"
)

func TestDirectSQLCanUseSystemKeychain(t *testing.T) {
	const target, ref = "postgres://database.example/jobs", "jobs"
	keyring.MockInit()
	t.Setenv(execution.SQLPasswordRefEnv, ref)
	t.Setenv(execution.SQLPasswordEnv, "")
	t.Setenv(execution.SQLTargetEnv, target)
	t.Setenv(execution.SQLUsernameEnv, "reader")
	t.Setenv("PROTOVOY_SQL_PASSWORD_KEYCHAIN", "")
	if err := secrets.Put(ref, "fixture-password"); err != nil {
		t.Fatal(err)
	}
	credentials := execution.SQLCredentials{PasswordRef: ref}
	if _, err := resolveDirectSQL(target, credentials); err == nil {
		t.Fatal("used the keychain without opt-in")
	}
	t.Setenv("PROTOVOY_SQL_PASSWORD_KEYCHAIN", "1")
	if _, err := resolveDirectSQL(target, credentials); err != nil {
		t.Fatal(err)
	}
	credentials.Username = "other"
	if _, err := resolveDirectSQL(target, credentials); err == nil {
		t.Fatal("accepted a different database username")
	}
}
