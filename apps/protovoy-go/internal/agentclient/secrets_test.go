package agentclient

import (
	"context"
	"net/http"
	"testing"
)

func TestSecretClientRejectsInvalidInputLocally(t *testing.T) {
	client := &Client{do: func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid secret input reached the network")
		return nil, nil
	}}
	ctx := context.Background()
	if err := client.PutSecret(ctx, "", "fixture"); err == nil {
		t.Fatal("empty reference accepted")
	}
	if err := client.PutSecret(ctx, "fixture", ""); err == nil {
		t.Fatal("empty value accepted")
	}
	if _, err := client.SecretExists(ctx, "invalid\nreference"); err == nil {
		t.Fatal("control character reference accepted")
	}
	if err := client.DeleteSecret(ctx, ""); err == nil {
		t.Fatal("empty deletion reference accepted")
	}
}
