//go:build !js

package collaboration

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const ssoPath = "/api/team/v1/auth/sso"
const ssoCookie = "__Host-protovoy-team-sso"
const ssoSessionCookie = "__Host-protovoy-team-handoff"

type OIDCConfig struct {
	Issuer, ClientID, ClientSecret, Organization, Role string
}

func (store *AccessStore) RegisterOIDCRoutes(ctx context.Context, mux *http.ServeMux, origin string, config OIDCConfig) error {
	enabled := config.Issuer != ""
	loginPath := ""
	if enabled {
		client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}}
		if err := store.registerOIDCFlow(ctx, mux, origin, config, client); err != nil {
			return err
		}
		loginPath = ssoPath + "/login"
	}
	mux.HandleFunc("GET /v1/auth/sso/config", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		writeWorkspaceResponse(w, struct {
			Enabled        bool   `json:"enabled"`
			LoginPath      string `json:"loginPath,omitempty"`
			DesktopEnabled bool   `json:"desktopEnabled"`
		}{enabled, loginPath, enabled}, nil)
	})
	return nil
}

func (store *AccessStore) registerOIDCFlow(ctx context.Context, mux *http.ServeMux, origin string, config OIDCConfig, client *http.Client) error {
	if !secureOIDCURL(origin) || !secureOIDCURL(config.Issuer) || config.ClientID == "" ||
		config.ClientSecret == "" || config.Organization == "" {
		return errors.New("team SSO requires hosted HTTPS, issuer, client credentials and organization ID")
	}
	if config.Role == "" {
		config.Role = "VIEWER"
	}
	if config.Role != "VIEWER" && config.Role != "RUNNER" && config.Role != "EDITOR" {
		return errors.New("team SSO requires a non-administrative default role")
	}
	provider, err := oidc.NewProvider(oidc.ClientContext(ctx, client), config.Issuer)
	if err != nil {
		return errors.New("could not discover the configured team OIDC provider")
	}
	endpoint := provider.Endpoint()
	var discovery struct {
		JWKS string `json:"jwks_uri"`
	}
	if provider.Claims(&discovery) != nil || !secureOIDCURL(endpoint.AuthURL) ||
		!secureOIDCURL(endpoint.TokenURL) || !secureOIDCURL(discovery.JWKS) {
		return errors.New("team OIDC provider endpoints must use HTTPS")
	}
	oauth := oauth2.Config{
		ClientID: config.ClientID, ClientSecret: config.ClientSecret, Endpoint: endpoint,
		RedirectURL: origin + ssoPath + "/callback", Scopes: []string{oidc.ScopeOpenID, "profile", "email"},
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: config.ClientID})
	desktopGrants := &desktopOIDCGrants{}
	mux.HandleFunc("POST /v1/auth/sso/desktop/session", desktopGrants.session)
	mux.HandleFunc("GET /v1/auth/sso/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		state, nonce, proof := rand.Text(), rand.Text(), oauth2.GenerateVerifier()
		cookie := state + "." + nonce + "." + proof
		query := r.URL.Query()
		if query.Has("desktopReturn") || query.Has("desktopChallenge") {
			desktop := desktopOIDCReturn{query.Get("desktopReturn"), query.Get("desktopChallenge")}
			if len(query["desktopReturn"]) != 1 || len(query["desktopChallenge"]) != 1 || desktop.validate() != nil {
				http.Error(w, "invalid desktop SSO request", http.StatusBadRequest)
				return
			}
			data, _ := json.Marshal(desktop)
			cookie += "." + base64.RawURLEncoding.EncodeToString(data)
		}
		setSSOCookie(w, cookie, 300)
		http.Redirect(w, r, oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(proof)), http.StatusFound)
	})
	mux.HandleFunc("GET /v1/auth/sso/callback", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		cookie, err := r.Cookie(ssoCookie)
		setSSOCookie(w, "", -1)
		if err != nil {
			http.Error(w, "team SSO session expired", http.StatusUnauthorized)
			return
		}
		parts := strings.Split(cookie.Value, ".")
		code := r.URL.Query().Get("code")
		if (len(parts) != 3 && len(parts) != 4) || parts[0] == "" || code == "" ||
			subtle.ConstantTimeCompare([]byte(parts[0]), []byte(r.URL.Query().Get("state"))) != 1 {
			http.Error(w, "invalid team SSO response", http.StatusUnauthorized)
			return
		}
		var desktop desktopOIDCReturn
		if len(parts) == 4 {
			data, err := base64.RawURLEncoding.DecodeString(parts[3])
			if err != nil || json.Unmarshal(data, &desktop) != nil || desktop.validate() != nil {
				http.Error(w, "invalid desktop SSO return", http.StatusUnauthorized)
				return
			}
		}
		ctx := oidc.ClientContext(r.Context(), client)
		token, err := oauth.Exchange(ctx, code, oauth2.VerifierOption(parts[2]))
		if err != nil {
			http.Error(w, "team SSO code exchange failed", http.StatusUnauthorized)
			return
		}
		raw, _ := token.Extra("id_token").(string)
		verified, err := verifier.Verify(ctx, raw)
		if err != nil || subtle.ConstantTimeCompare([]byte(verified.Nonce), []byte(parts[1])) != 1 {
			http.Error(w, "invalid team SSO identity", http.StatusUnauthorized)
			return
		}
		var claims struct {
			Email         string `json:"email"`
			Name          string `json:"name"`
			EmailVerified bool   `json:"email_verified"`
		}
		if verified.Claims(&claims) != nil {
			http.Error(w, "invalid team SSO claims", http.StatusUnauthorized)
			return
		}
		identity := federatedIdentity{verified.Issuer, verified.Subject, claims.Email, claims.Name, claims.EmailVerified}
		result, err := store.loginFederated(ctx, identity, config.Organization, config.Role)
		if err != nil {
			http.Error(w, "team SSO account is unavailable", http.StatusForbidden)
			return
		}
		if desktop.URI != "" {
			code, err := desktopGrants.issue(result, desktop.Challenge)
			if err != nil {
				http.Error(w, "desktop SSO capacity is busy", http.StatusServiceUnavailable)
				return
			}
			location, _ := url.Parse(desktop.URI)
			query := location.Query()
			query.Set("code", code)
			location.RawQuery = query.Encode()
			http.Redirect(w, r, location.String(), http.StatusFound)
			return
		}
		payload, _ := json.Marshal(result)
		http.SetCookie(w, ssoHandoffCookie(base64.RawURLEncoding.EncodeToString(payload), 60))
		http.Redirect(w, r, origin+"/#team-sso", http.StatusFound)
	})
	mux.HandleFunc("POST /v1/auth/sso/session", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Header.Get("Origin") != origin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		cookie, err := r.Cookie(ssoSessionCookie)
		http.SetCookie(w, ssoHandoffCookie("", -1))
		if err != nil {
			http.Error(w, "team SSO handoff expired", http.StatusUnauthorized)
			return
		}
		payload, err := base64.RawURLEncoding.DecodeString(cookie.Value)
		var result AuthResult
		if err != nil || json.Unmarshal(payload, &result) != nil || result.Token == "" {
			http.Error(w, "invalid team SSO handoff", http.StatusUnauthorized)
			return
		}
		writeWorkspaceResponse(w, result, nil)
	})
	return nil
}

func ssoHandoffCookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{Name: ssoSessionCookie, Value: value, Path: "/", MaxAge: maxAge,
		Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode}
}

func setSSOCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: ssoCookie, Value: value, Path: "/", MaxAge: maxAge,
		Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}

func secureOIDCURL(value string) bool {
	u, err := url.Parse(value)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && !strings.Contains(value, "#")
}
