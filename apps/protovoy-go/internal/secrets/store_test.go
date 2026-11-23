//go:build !js

package secrets

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestSystemCredentialStore(t *testing.T) {
	if os.Getenv("PROTOVOY_TEST_SYSTEM_KEYCHAIN") != "1" {
		t.Skip("system credential store integration is opt-in")
	}
	reference := "integration-" + hex.EncodeToString([]byte(rand.Text()))
	value := rand.Text()
	t.Cleanup(func() {
		if err := Delete(reference); err != nil {
			t.Error("could not clean up temporary credential")
		}
	})
	if err := Put(reference, value); err != nil {
		t.Fatal(err)
	}
	if restored, err := Resolve(reference); err != nil || restored != value {
		t.Fatal("system credential store did not return the saved value")
	}
	if err := Delete(reference); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(reference); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted system credential remained readable")
	}
}

func TestSecretReferenceLifecycle(t *testing.T) {
	keyring.MockInit()
	const value = "fixture-secret-value"
	if err := Put("fixture", value); err != nil {
		t.Fatal(err)
	}
	if actual, err := Resolve("fixture"); err != nil || actual != value {
		t.Fatal("secret reference did not resolve")
	}
	if err := Delete("fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve("fixture"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted reference returned %v", err)
	}
	if err := Put("", value); err == nil {
		t.Fatal("empty reference was accepted")
	}
}
