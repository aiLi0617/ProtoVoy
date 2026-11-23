package execution

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestOAuthTokenGrantFormsAndCredentials(t *testing.T) {
	for _, grant := range []string{"client_credentials", "authorization_code"} {
		t.Run(grant, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.ParseForm() != nil || r.Form.Get("grant_type") != grant {
					t.Error("invalid grant form")
				}
				id, secret, ok := r.BasicAuth()
				if !ok || id != url.QueryEscape("client:id") || secret != url.QueryEscape("secret + value") {
					t.Error("client authentication mismatch")
				}
				if grant == "client_credentials" {
					if r.Form.Get("scope") != "read write" || r.Form.Get("audience") != "fixture" {
						t.Error("scope/audience missing")
					}
				} else if r.Form.Get("client_id") != "client:id" || r.Form.Get("code") != "code" || r.Form.Get("code_verifier") != strings.Repeat("v", 43) || r.Form.Get("redirect_uri") != "http://127.0.0.1:4000/callback" {
					t.Error("authorization-code parameters mismatch")
				}
				fmt.Fprint(w, `{"access_token":"fixture-token","token_type":"Bearer","expires_in":3600}`)
			}))
			defer server.Close()
			input := OAuthTokenRequest{Endpoint: server.URL, GrantType: grant, ClientID: "client:id", ClientSecret: "secret + value", Scope: "read write", Audience: "fixture", Code: "code", Verifier: strings.Repeat("v", 43), RedirectURI: "http://127.0.0.1:4000/callback"}
			token, err := (OAuthClient{Transport: server.Client().Transport}).Token(t.Context(), input)
			if err != nil || token.AccessToken != "fixture-token" || token.TokenType != "Bearer" {
				t.Fatalf("exchange failed: %+v %v", token, err)
			}
		})
	}
}

func TestOAuthRejectsRedirectsMalformedResponsesAndSecretDiagnostics(t *testing.T) {
	for _, test := range []struct {
		status int
		body   string
	}{
		{302, "secret diagnostic"}, {400, "secret diagnostic"}, {200, `{"access_token":"token\r\nInjected: value"}`},
		{200, `{"access_token":"token","token_type":"Bearer token"}`}, {200, `{"access_token":""}`}, {200, strings.Repeat("x", (64<<10)+1)},
	} {
		var hits atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			w.Header().Set("Location", "/redirected")
			w.WriteHeader(test.status)
			fmt.Fprint(w, test.body)
		}))
		_, err := (OAuthClient{}).Token(t.Context(), OAuthTokenRequest{Endpoint: server.URL, GrantType: "client_credentials", ClientID: "client", ClientSecret: "secret"})
		server.Close()
		if err == nil || strings.Contains(err.Error(), "diagnostic") || hits.Load() != 1 {
			t.Fatalf("unsafe response handling: %v hits=%d", err, hits.Load())
		}
	}
}

func TestOAuthValidationAndDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer server.Close()
	input := OAuthTokenRequest{Endpoint: server.URL, GrantType: "client_credentials", ClientID: "client", ClientSecret: "secret", Timeout: 25 * time.Millisecond}
	if _, err := (OAuthClient{}).Token(t.Context(), input); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline: %v", err)
	}
	for _, endpoint := range []string{"http://remote.test/token", "https://user:secret@host.test/token", "https://host.test/token#fragment"} {
		input.Endpoint = endpoint
		if _, err := (OAuthClient{}).Token(t.Context(), input); err == nil {
			t.Fatal("unsafe endpoint accepted")
		}
	}
}
