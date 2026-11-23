package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// OAuthTokenRequest is runtime-only input. Secrets, authorization codes and PKCE
// verifiers must be resolved by the host, never persisted in request documents.
type OAuthTokenRequest struct {
	Endpoint, GrantType, ClientID, ClientSecret  string
	Code, Verifier, RedirectURI, Scope, Audience string
	Timeout                                      time.Duration
}

type OAuthToken struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

type OAuthClient struct{ Transport http.RoundTripper }

func (client OAuthClient) Token(ctx context.Context, input OAuthTokenRequest) (OAuthToken, error) {
	form, err := prepareOAuthTokenForm(input)
	if err != nil {
		return OAuthToken{}, err
	}
	invalid := errors.New("invalid OAuth token request")
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, input.Endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return OAuthToken{}, invalid
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	if input.ClientSecret != "" {
		request.SetBasicAuth(url.QueryEscape(input.ClientID), url.QueryEscape(input.ClientSecret))
	}
	timeout := input.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	httpClient := http.Client{Transport: client.Transport, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := httpClient.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return OAuthToken{}, ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return OAuthToken{}, context.DeadlineExceeded
		}
		return OAuthToken{}, errors.New("OAuth token endpoint request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return OAuthToken{}, fmt.Errorf("OAuth token endpoint returned HTTP %d", response.StatusCode)
	}
	const limit = 64 << 10
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if ctx.Err() != nil {
		return OAuthToken{}, ctx.Err()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return OAuthToken{}, context.DeadlineExceeded
	}
	var token OAuthToken
	if err != nil || len(data) > limit || json.Unmarshal(data, &token) != nil {
		return OAuthToken{}, errors.New("invalid OAuth token response")
	}
	if token.TokenType == "" {
		token.TokenType = "Bearer"
	}
	if !validOAuthHeaderPart(token.AccessToken, 16<<10) || !validOAuthHeaderPart(token.TokenType, 64) {
		return OAuthToken{}, errors.New("invalid OAuth token response")
	}
	return token, nil
}

func validOAuthVerifier(value string) bool {
	return len(value) >= 43 && len(value) <= 128 && !strings.ContainsFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-._~", r))
	})
}

func validOAuthHeaderPart(value string, limit int) bool {
	return len(value) > 0 && len(value) <= limit && !strings.ContainsFunc(value, func(r rune) bool { return r < 33 || r > 126 })
}

func (input OAuthTokenRequest) Validate() error { _, err := prepareOAuthTokenForm(input); return err }
func prepareOAuthTokenForm(input OAuthTokenRequest) (url.Values, error) {
	invalid := errors.New("invalid OAuth token request")
	endpoint, err := url.Parse(input.Endpoint)
	if err != nil || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.Fragment != "" {
		return nil, invalid
	}
	ip, _ := netip.ParseAddr(endpoint.Hostname())
	if endpoint.Scheme != "https" && !(endpoint.Scheme == "http" && (ip.IsLoopback() || strings.EqualFold(endpoint.Hostname(), "localhost"))) {
		return nil, errors.New("OAuth token endpoint requires HTTPS or loopback HTTP")
	}
	for _, value := range []string{input.Endpoint, input.ClientID, input.ClientSecret, input.Code, input.RedirectURI, input.Scope, input.Audience} {
		if len(value) > 8192 || strings.ContainsAny(value, "\x00\r\n") {
			return nil, invalid
		}
	}
	if input.ClientID == "" || input.Timeout < 0 || input.Timeout > 5*time.Minute {
		return nil, invalid
	}
	form := url.Values{"grant_type": {input.GrantType}}
	switch input.GrantType {
	case "client_credentials":
		if input.ClientSecret == "" {
			return nil, invalid
		}
		if input.Scope != "" {
			form.Set("scope", input.Scope)
		}
		if input.Audience != "" {
			form.Set("audience", input.Audience)
		}
	case "authorization_code":
		if input.Code == "" || input.RedirectURI == "" || !validOAuthVerifier(input.Verifier) {
			return nil, invalid
		}
		form.Set("client_id", input.ClientID)
		form.Set("code", input.Code)
		form.Set("redirect_uri", input.RedirectURI)
		form.Set("code_verifier", input.Verifier)
	default:
		return nil, invalid
	}
	return form, nil
}
