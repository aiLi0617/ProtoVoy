//go:build !js

package collaboration

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"protovoy.dev/protovoy/internal/teamclient"
)

func TestOIDCLoginStartsPKCEAndRejectsWrongState(t *testing.T) {
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			t.Error("mismatched state reached the identity provider")
			http.NotFound(w, r)
			return
		}
		issuer := "https://" + r.Host
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer": issuer, "authorization_endpoint": issuer + "/authorize",
			"token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys",
		})
	}))
	defer provider.Close()
	mux := http.NewServeMux()
	store := &AccessStore{}
	const origin = "https://workbench.example.test"
	config := OIDCConfig{Issuer: provider.URL, ClientID: "client", ClientSecret: "secret", Organization: "team"}
	if err := store.registerOIDCFlow(t.Context(), mux, origin, config, provider.Client()); err != nil {
		t.Fatal(err)
	}
	start := httptest.NewRecorder()
	mux.ServeHTTP(start, httptest.NewRequest("GET", origin+"/v1/auth/sso/login", nil))
	location, err := url.Parse(start.Header().Get("Location"))
	if err != nil || start.Code != http.StatusFound {
		t.Fatalf("SSO login did not redirect: %v, %d", err, start.Code)
	}
	query := location.Query()
	if query.Get("state") == "" || query.Get("nonce") == "" || query.Get("code_challenge") == "" ||
		query.Get("code_challenge_method") != "S256" || query.Get("redirect_uri") != origin+ssoPath+"/callback" {
		t.Fatal("SSO authorization omitted state, nonce, PKCE or the configured callback")
	}
	cookies := start.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatal("SSO login did not set a secure flow cookie")
	}
	callback := httptest.NewRequest("GET", origin+"/v1/auth/sso/callback?code=test&state=wrong", nil)
	callback.AddCookie(cookies[0])
	result := httptest.NewRecorder()
	mux.ServeHTTP(result, callback)
	if result.Code != http.StatusUnauthorized || result.Result().Cookies()[0].MaxAge != -1 {
		t.Fatal("SSO callback accepted the wrong state or retained the flow cookie")
	}
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"token":"fixture","organizationId":"team"}`))
	for _, test := range []struct {
		origin string
		status int
	}{{"https://other.example.test", 403}, {origin, 200}} {
		req := httptest.NewRequest("POST", origin+"/v1/auth/sso/session", nil)
		req.Header.Set("Origin", test.origin)
		req.AddCookie(ssoHandoffCookie(payload, 60))
		result := httptest.NewRecorder()
		mux.ServeHTTP(result, req)
		if result.Code != test.status || result.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("handoff from %s returned %d", test.origin, result.Code)
		}
		if test.status == 200 && result.Result().Cookies()[0].MaxAge != -1 {
			t.Fatal("handoff cookie was not cleared")
		}
	}
}

func TestOIDCSignedCallbackCreatesUsableTeamSession(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	encode := base64.RawURLEncoding.EncodeToString
	type authorization struct{ nonce, challenge string }
	flow := make(chan authorization, 1)
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issuer := "https://" + r.Host
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			json.NewEncoder(w).Encode(map[string]string{"issuer": issuer, "authorization_endpoint": issuer + "/authorize",
				"token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys"})
		case "/keys":
			json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{
				{"kty": "RSA", "kid": "fixture", "alg": "RS256", "n": encode(key.N.Bytes()), "e": "AQAB"},
			}})
		case "/token":
			var pending authorization
			select {
			case pending = <-flow:
			default:
				http.Error(w, "unexpected exchange", http.StatusBadRequest)
				return
			}
			proof := sha256.Sum256([]byte(r.FormValue("code_verifier")))
			client, secret, ok := r.BasicAuth()
			if !ok || client != "client" || secret != "secret" || encode(proof[:]) != pending.challenge || r.FormValue("code") != "fixture-code" {
				http.Error(w, "invalid exchange", http.StatusBadRequest)
				return
			}
			claims, _ := json.Marshal(map[string]any{"iss": issuer, "sub": "member", "aud": "client",
				"iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix(), "nonce": pending.nonce,
				"email": "member@example.test", "email_verified": true})
			unsigned := encode([]byte(`{"alg":"RS256","kid":"fixture"}`)) + "." + encode(claims)
			digest := sha256.Sum256([]byte(unsigned))
			signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
			if err != nil {
				t.Error(err)
				http.Error(w, "could not sign fixture", 500)
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"access_token": "fixture-access", "token_type": "Bearer", "id_token": unsigned + "." + encode(signature)})
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "team.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := NewAccessStore(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.Bootstrap(t.Context(), "owner@example.test", "test-owner-password", "Owner", "Team")
	if err != nil {
		t.Fatal(err)
	}
	const origin = "https://workbench.example.test"
	mux := http.NewServeMux()
	config := OIDCConfig{Issuer: provider.URL, ClientID: "client", ClientSecret: "secret", Organization: owner.OrganizationID}
	if err := store.registerOIDCFlow(t.Context(), mux, origin, config, provider.Client()); err != nil {
		t.Fatal(err)
	}
	start := httptest.NewRecorder()
	mux.ServeHTTP(start, httptest.NewRequest("GET", origin+"/v1/auth/sso/login", nil))
	location, err := url.Parse(start.Header().Get("Location"))
	if err != nil || start.Code != http.StatusFound {
		t.Fatalf("login start failed: %v", err)
	}
	query := location.Query()
	flow <- authorization{query.Get("nonce"), query.Get("code_challenge")}
	callback := httptest.NewRequest("GET", origin+"/v1/auth/sso/callback?code=fixture-code&state="+url.QueryEscape(query.Get("state")), nil)
	callback.AddCookie(start.Result().Cookies()[0])
	redirect := httptest.NewRecorder()
	mux.ServeHTTP(redirect, callback)
	if redirect.Code != http.StatusFound || redirect.Header().Get("Location") != origin+"/#team-sso" {
		t.Fatalf("callback returned %d: %s", redirect.Code, redirect.Body)
	}
	handoff := httptest.NewRequest("POST", origin+"/v1/auth/sso/session", nil)
	handoff.Header.Set("Origin", origin)
	for _, cookie := range redirect.Result().Cookies() {
		if cookie.Name == ssoSessionCookie {
			handoff.AddCookie(cookie)
		}
	}
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, handoff)
	var session AuthResult
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &session) != nil ||
		session.User.Email != "member@example.test" || session.Role != "VIEWER" || strings.TrimSpace(session.Token) == "" {
		t.Fatalf("handoff failed with status %d", response.Code)
	}
	handoff.Header.Set("Authorization", "Bearer "+session.Token)
	if actor, denial := store.WorkspaceAccess(handoff, owner.OrganizationID, false); denial != 0 || actor != session.User.ID {
		t.Fatalf("issued session could not read workspace: %d", denial)
	}
	// The native flow uses the same signed OIDC identity verification but never
	// puts the team session token in the browser's redirect or handoff cookie.
	proof := encode(make([]byte, 32))
	challenge := sha256.Sum256([]byte(proof))
	nativeState := encode([]byte("01234567890123456789012345678901"))
	returnURI := "http://127.0.0.1:43821/team-sso?state=" + nativeState
	parameters := url.Values{"desktopReturn": {returnURI}, "desktopChallenge": {encode(challenge[:])}}
	start = httptest.NewRecorder()
	mux.ServeHTTP(start, httptest.NewRequest("GET", origin+"/v1/auth/sso/login?"+parameters.Encode(), nil))
	location, err = url.Parse(start.Header().Get("Location"))
	if err != nil || start.Code != http.StatusFound {
		t.Fatal("native login did not start")
	}
	query = location.Query()
	flow <- authorization{query.Get("nonce"), query.Get("code_challenge")}
	callback = httptest.NewRequest("GET", origin+"/v1/auth/sso/callback?code=fixture-code&state="+url.QueryEscape(query.Get("state")), nil)
	callback.AddCookie(start.Result().Cookies()[0])
	redirect = httptest.NewRecorder()
	mux.ServeHTTP(redirect, callback)
	nativeURL, err := url.Parse(redirect.Header().Get("Location"))
	if err != nil || redirect.Code != http.StatusFound || nativeURL.Host != "127.0.0.1:43821" || nativeURL.Query().Get("state") != nativeState || nativeURL.Query().Get("code") == "" {
		t.Fatal("native callback did not return a loopback authorization code")
	}
	for _, cookie := range redirect.Result().Cookies() {
		if cookie.Name == ssoSessionCookie {
			t.Fatal("native callback exposed a browser session cookie")
		}
	}
	exchangeData, _ := json.Marshal(map[string]string{"code": nativeURL.Query().Get("code"), "proof": proof})
	exchange := httptest.NewRequest("POST", origin+"/v1/auth/sso/desktop/session", strings.NewReader(string(exchangeData)))
	nativeResponse := httptest.NewRecorder()
	mux.ServeHTTP(nativeResponse, exchange)
	var nativeSession AuthResult
	if nativeResponse.Code != 200 || json.Unmarshal(nativeResponse.Body.Bytes(), &nativeSession) != nil || nativeSession.Token == "" || nativeSession.User.ID != session.User.ID {
		t.Fatalf("native proof exchange failed: HTTP %d", nativeResponse.Code)
	}
	if strings.Contains(redirect.Header().Get("Location"), nativeSession.Token) {
		t.Fatal("native access token was exposed in a URL")
	}
	exchange.Header.Set("Authorization", "Bearer "+nativeSession.Token)
	if actor, denial := store.WorkspaceAccess(exchange, owner.OrganizationID, false); denial != 0 || actor != nativeSession.User.ID {
		t.Fatal("native session could not access team workspace")
	}
	replay := httptest.NewRecorder()
	mux.ServeHTTP(replay, httptest.NewRequest("POST", origin+"/v1/auth/sso/desktop/session", strings.NewReader(string(exchangeData))))
	if replay.Code != http.StatusUnauthorized {
		t.Fatal("native authorization code could be replayed")
	}
	// Exercise the native client's real listener and HTTPS redemption against
	// this signed provider flow, rather than a stubbed desktop session response.
	nativeMux := http.NewServeMux()
	nativeServer := httptest.NewTLSServer(http.StripPrefix("/api/team", nativeMux))
	defer nativeServer.Close()
	if err := store.registerOIDCFlow(t.Context(), nativeMux, nativeServer.URL, config, provider.Client()); err != nil {
		t.Fatal(err)
	}
	nativeMux.HandleFunc("GET /v1/auth/sso/config", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"enabled":true,"desktopEnabled":true}`))
	})
	nativeClient, err := teamclient.New(nativeServer.URL, nativeServer.Client().Transport)
	if err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Transport: nativeServer.Client().Transport, Jar: jar, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	err = nativeClient.CompleteDesktopSSO(t.Context(), func(loginURL string) error {
		start, err := browser.Get(loginURL)
		if err != nil {
			return err
		}
		start.Body.Close()
		authorize, err := url.Parse(start.Header.Get("Location"))
		if err != nil || start.StatusCode != http.StatusFound {
			return errors.New("native browser login did not start")
		}
		parameters := authorize.Query()
		flow <- authorization{parameters.Get("nonce"), parameters.Get("code_challenge")}
		callbackURL := parameters.Get("redirect_uri") + "?code=fixture-code&state=" + url.QueryEscape(parameters.Get("state"))
		callback, err := browser.Get(callbackURL)
		if err != nil {
			return err
		}
		callback.Body.Close()
		if callback.StatusCode != http.StatusFound {
			return errors.New("signed native callback failed")
		}
		local, err := browser.Get(callback.Header.Get("Location"))
		if err != nil {
			return err
		}
		local.Body.Close()
		if local.StatusCode != http.StatusOK {
			return errors.New("native loopback rejected signed callback")
		}
		return nil
	})
	if err != nil || nativeClient.Organization != owner.OrganizationID || nativeClient.UserID != session.User.ID || nativeClient.Role != "VIEWER" {
		t.Fatalf("complete native SSO flow failed: %v", err)
	}
}
