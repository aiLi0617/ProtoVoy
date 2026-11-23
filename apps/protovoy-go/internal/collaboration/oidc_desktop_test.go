//go:build !js

package collaboration

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDesktopOIDCReturnRejectsRemoteAndAmbiguousCallbacks(t *testing.T) {
	state := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	valid := desktopOIDCReturn{"http://127.0.0.1:43821/team-sso?state=" + state, state}
	if err := valid.validate(); err != nil {
		t.Fatal(err)
	}
	for _, uri := range []string{
		"https://remote.example/team-sso?state=" + state,
		"http://localhost:43821/team-sso?state=" + state,
		"http://127.0.0.1:80/team-sso?state=" + state,
		"http://127.0.0.1:43821/other?state=" + state,
		valid.URI + "&state=" + state,
		valid.URI + "&redirect=https://remote.example",
		valid.URI + "#fragment",
		"http://user@127.0.0.1:43821/team-sso?state=" + state,
	} {
		candidate := valid
		candidate.URI = uri
		if candidate.validate() == nil {
			t.Fatal("accepted an unsafe desktop callback")
		}
	}
	valid.Challenge = "not-a-proof"
	if valid.validate() == nil {
		t.Fatal("accepted malformed proof challenge")
	}
}

func TestDesktopOIDCGrantsAreBoundedExpiringAndProofBound(t *testing.T) {
	proof := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	digest := sha256.Sum256([]byte(proof))
	challenge := base64.RawURLEncoding.EncodeToString(digest[:])
	grants := &desktopOIDCGrants{}
	code, err := grants.issue(AuthResult{Token: "fixture-session"}, challenge)
	if err != nil {
		t.Fatal(err)
	}
	wrong := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	if _, err := grants.redeem(code, wrong); err == nil {
		t.Fatal("redeemed grant without the native proof")
	}
	data, _ := json.Marshal(map[string]string{"code": code, "proof": proof})
	for _, origin := range []string{"https://workbench.example.test", "null"} {
		request := httptest.NewRequest(http.MethodPost, "https://workbench.example.test/v1/auth/sso/desktop/session", bytes.NewReader(data))
		request.Header.Set("Origin", origin)
		response := httptest.NewRecorder()
		grants.session(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatal("browser could redeem a native grant")
		}
	}
	result, err := grants.redeem(code, proof)
	if err != nil || result.Token != "fixture-session" {
		t.Fatal("correct proof did not redeem grant")
	}
	if _, err := grants.redeem(code, proof); err == nil {
		t.Fatal("replayed a consumed grant")
	}
	for range 128 {
		if _, err := grants.issue(AuthResult{Token: "fixture-session"}, challenge); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := grants.issue(AuthResult{Token: "fixture-session"}, challenge); err == nil {
		t.Fatal("exceeded grant capacity")
	}
	expiredCode := ""
	for pendingCode, grant := range grants.values {
		grant.expires = time.Now().Add(-time.Second)
		grants.values[pendingCode] = grant
		expiredCode = pendingCode
	}
	if _, err := grants.redeem(expiredCode, proof); err == nil {
		t.Fatal("redeemed expired grant")
	}
	if _, err := grants.issue(AuthResult{Token: "fixture-session"}, challenge); err != nil {
		t.Fatal("expired grants did not release capacity")
	}
	request := httptest.NewRequest(http.MethodPost, "https://workbench.example.test/v1/auth/sso/desktop/session", strings.NewReader(`{"unknown":true}`))
	response := httptest.NewRecorder()
	grants.session(response, request)
	if response.Code != http.StatusBadRequest || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("malformed exchange was accepted or cacheable")
	}
}
