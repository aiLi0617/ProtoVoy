package agentclient

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"protovoy.dev/protovoy/internal/execution"
)

type httpAuthKey struct{}

func (client *Client) RunHTTPWithSecret(ctx context.Context, target string, payload execution.HTTPPayload, reference, header string, publish func(Response)) (Response, error) {
	if reference == "" || !execution.ValidPasswordReference(reference) || header == "" || payload.FollowRedirects {
		return Response{}, errors.New("HTTP credential requires a reference, header and redirects disabled")
	}
	ctx = context.WithValue(ctx, httpAuthKey{}, map[string]string{
		"type": "header", "secretRef": reference, "header": header,
	})
	return client.RunHTTP(ctx, target, payload, publish)
}

func (client *Client) PutHTTPSecret(ctx context.Context, reference, target, header, value string) error {
	if reference == "" || !execution.ValidPasswordReference(reference) {
		return errors.New("invalid HTTP credential reference")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	return client.json(ctx, http.MethodPut, "/v1/http-secrets/"+url.PathEscape(reference), map[string]string{
		"target": target, "header": header, "value": value,
	}, nil)
}

func (client *Client) DeleteHTTPSecret(ctx context.Context, reference string) error {
	if reference == "" || !execution.ValidPasswordReference(reference) {
		return errors.New("invalid HTTP credential reference")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	return client.json(ctx, http.MethodDelete, "/v1/http-secrets/"+url.PathEscape(reference), nil, nil)
}
