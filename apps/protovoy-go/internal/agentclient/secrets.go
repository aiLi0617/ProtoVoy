package agentclient

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"protovoy.dev/protovoy/internal/execution"
)

func (client *Client) SupportsSecrets() bool {
	return client.secretsVersion == "1"
}

func (client *Client) PutSecret(ctx context.Context, reference, value string) error {
	if value == "" || len(value) > 2560 {
		return errors.New("secret value requires 1 to 2560 bytes")
	}
	return client.secretRequest(ctx, http.MethodPut, reference, map[string]string{"value": value}, nil)
}

func (client *Client) SecretExists(ctx context.Context, reference string) (bool, error) {
	var result struct {
		Exists bool `json:"exists"`
	}
	err := client.secretRequest(ctx, http.MethodGet, reference, nil, &result)
	return result.Exists, err
}

func (client *Client) DeleteSecret(ctx context.Context, reference string) error {
	return client.secretRequest(ctx, http.MethodDelete, reference, nil, nil)
}

func (client *Client) secretRequest(ctx context.Context, method, reference string, input, output any) error {
	if reference == "" || !execution.ValidPasswordReference(reference) {
		return errors.New("invalid secret reference")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	return client.json(ctx, method, "/v1/secrets/"+url.PathEscape(reference), input, output)
}
