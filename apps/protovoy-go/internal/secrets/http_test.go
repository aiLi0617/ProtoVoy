//go:build !js

package secrets

import (
	"testing"

	"github.com/zalando/go-keyring"
)

func TestHTTPCredentialRequiresExactDestination(t *testing.T) {
	keyring.MockInit()
	const target = "https://example.test/api?scope=one"
	if err := PutHTTP("fixture", target, "Authorization", "Bearer fixture"); err != nil {
		t.Fatal(err)
	}
	if value, err := ResolveHTTP("fixture", target, "authorization"); err != nil || value != "Bearer fixture" {
		t.Fatal("matching HTTP credential did not resolve")
	}
	for _, destination := range []string{"https://other.test/api?scope=one", "https://example.test/api?scope=two", "http://example.test/api?scope=one"} {
		if _, err := ResolveHTTP("fixture", destination, "Authorization"); err == nil {
			t.Fatal("credential resolved for a different destination")
		}
	}
	if _, err := ResolveHTTP("fixture", target, "X-Api-Key"); err == nil {
		t.Fatal("credential resolved for a different header")
	}
}
