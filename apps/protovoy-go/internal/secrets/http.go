//go:build !js

package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/zalando/go-keyring"
	"protovoy.dev/protovoy/internal/execution"
)

const httpService = service + ".http"

// HTTPHeaders applies a binding to a request with an already resolved target.
func HTTPHeaders(headers [][2]string, target, reference, header string, followRedirects bool) ([][2]string, error) {
	return httpHeaders(headers, target, reference, header, followRedirects, ResolveHTTP)
}

func HTTPHeadersContext(ctx context.Context, headers [][2]string, target, reference, header string, followRedirects bool) ([][2]string, error) {
	return httpHeaders(headers, target, reference, header, followRedirects, func(reference, target, header string) (string, error) {
		return ResolveHTTPContext(ctx, reference, target, header)
	})
}

func httpHeaders(headers [][2]string, target, reference, header string, followRedirects bool, resolve func(string, string, string) (string, error)) ([][2]string, error) {
	if followRedirects {
		return nil, errors.New("HTTP credential bindings require redirects disabled")
	}
	for _, pair := range headers {
		if strings.EqualFold(pair[0], header) {
			return nil, errors.New("credential header is already present in the request")
		}
	}
	value, err := resolve(reference, target, header)
	if err != nil {
		return nil, err
	}
	return append(headers, [2]string{header, value}), nil
}

func HTTPExists(reference string) (bool, error) {
	if !validReference(reference) {
		return false, errors.New("invalid HTTP credential reference")
	}
	_, err := keyring.Get(httpService, reference)
	if errors.Is(err, keyring.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, errors.New("could not read HTTP credential from the system store")
	}
	return true, nil
}

func DeleteHTTP(reference string) error {
	if !validReference(reference) {
		return errors.New("invalid HTTP credential reference")
	}
	if err := keyring.Delete(httpService, reference); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return errors.New("could not delete HTTP credential from the system store")
	}
	return nil
}

type httpCredential struct {
	Target string
	Header string
	Value  string
	OAuth  *execution.OAuthTokenRequest `json:",omitempty"`
}

func PutHTTP(reference, target, header, value string) error {
	parsed, err := url.Parse(target)
	if !validReference(reference) || err != nil || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("HTTP credential requires a valid reference and absolute HTTP target")
	}
	header = http.CanonicalHeaderKey(header)
	headers, err := execution.ParseHeaders(header + ": " + value)
	if err != nil || len(headers) != 1 || strings.ContainsAny(header+value, "\r\n") || value == "" {
		return errors.New("HTTP credential requires a valid single request header")
	}
	data, err := json.Marshal(httpCredential{Target: target, Header: header, Value: value})
	if err != nil || len(data) > 2560 {
		return errors.New("HTTP credential exceeds system credential storage limit")
	}
	if keyring.Set(httpService, reference, string(data)) != nil {
		return errors.New("could not write HTTP credential to the system store")
	}
	return nil
}

func ResolveHTTP(reference, target, header string) (string, error) {
	credential, err := readHTTP(reference, target, header)
	if err != nil {
		return "", err
	}
	if credential.OAuth != nil {
		return "", errors.New("OAuth HTTP binding requires context-aware execution")
	}
	return credential.Value, nil
}

// ValidateHTTPBinding checks the exact resource binding without contacting an
// OAuth provider. Hosts can safely call it before registering a cancellable job.
func ValidateHTTPBinding(reference, target, header string) error {
	credential, err := readHTTP(reference, target, header)
	if err != nil {
		return err
	}
	if credential.OAuth != nil {
		if credential.Value != "" || credential.Header != "Authorization" {
			return errors.New("invalid OAuth HTTP binding")
		}
		return credential.OAuth.Validate()
	}
	return nil
}

func readHTTP(reference, target, header string) (httpCredential, error) {
	if !validReference(reference) {
		return httpCredential{}, errors.New("invalid HTTP credential reference")
	}
	data, err := keyring.Get(httpService, reference)
	if err != nil {
		return httpCredential{}, errors.New("could not read HTTP credential from the system store")
	}
	var credential httpCredential
	if json.Unmarshal([]byte(data), &credential) != nil || credential.Target != target ||
		credential.Header != http.CanonicalHeaderKey(header) {
		return httpCredential{}, errors.New("HTTP credential target or header does not match")
	}
	return credential, nil
}

// PutHTTPOAuth stores exchange inputs only in the OS credential store. The
// resource target is bound independently of the token endpoint.
func PutHTTPOAuth(reference, target string, input execution.OAuthTokenRequest) error {
	parsed, err := url.Parse(target)
	if !validReference(reference) || err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("OAuth HTTP binding requires a valid reference and resource target")
	}
	if err = input.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(httpCredential{Target: target, Header: "Authorization", OAuth: &input})
	if err != nil || len(data) > 2560 {
		return errors.New("OAuth HTTP binding exceeds system credential storage limit")
	}
	if keyring.Set(httpService, reference, string(data)) != nil {
		return errors.New("could not write OAuth HTTP binding to the system store")
	}
	return nil
}

func ResolveHTTPContext(ctx context.Context, reference, target, header string) (string, error) {
	credential, err := readHTTP(reference, target, header)
	if err != nil {
		return "", err
	}
	if credential.OAuth == nil {
		return credential.Value, nil
	}
	if credential.Value != "" || credential.Header != "Authorization" {
		return "", errors.New("invalid OAuth HTTP binding")
	}
	token, err := (execution.OAuthClient{}).Token(ctx, *credential.OAuth)
	if err != nil {
		return "", err
	}
	return token.TokenType + " " + token.AccessToken, nil
}
