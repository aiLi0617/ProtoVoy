//go:build !js

package collaboration

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

type desktopOIDCReturn struct {
	URI       string `json:"uri"`
	Challenge string `json:"challenge"`
}

func validDesktopProof(value string) bool {
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(value)
	return err == nil && len(decoded) == 32 && base64.RawURLEncoding.EncodeToString(decoded) == value
}

func (d desktopOIDCReturn) validate() error {
	u, err := url.Parse(d.URI)
	if err != nil {
		return errors.New("invalid desktop SSO return")
	}
	port, err := strconv.Atoi(u.Port())
	query, queryErr := url.ParseQuery(u.RawQuery)
	if err != nil || port < 1024 || port > 65535 || u.Scheme != "http" || u.Host != "127.0.0.1:"+strconv.Itoa(port) || u.User != nil ||
		u.Path != "/team-sso" || u.RawPath != "" || u.Fragment != "" || queryErr != nil || len(query) != 1 || len(query["state"]) != 1 ||
		!validDesktopProof(query.Get("state")) || !validDesktopProof(d.Challenge) || len(d.URI) > 256 {
		return errors.New("desktop SSO requires a numeric loopback callback and a valid proof challenge")
	}
	return nil
}

type desktopOIDCGrant struct {
	result    AuthResult
	challenge string
	expires   time.Time
}

type desktopOIDCGrants struct {
	mu     sync.Mutex
	values map[string]desktopOIDCGrant
}

func (g *desktopOIDCGrants) prune(now time.Time) {
	for code, value := range g.values {
		if !now.Before(value.expires) {
			delete(g.values, code)
		}
	}
}

func (g *desktopOIDCGrants) issue(result AuthResult, challenge string) (string, error) {
	if !validDesktopProof(challenge) {
		return "", errors.New("invalid desktop SSO challenge")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.prune(time.Now())
	if len(g.values) >= 128 {
		return "", errors.New("desktop SSO capacity is busy")
	}
	if g.values == nil {
		g.values = map[string]desktopOIDCGrant{}
	}
	code := rand.Text()
	g.values[code] = desktopOIDCGrant{result, challenge, time.Now().Add(time.Minute)}
	return code, nil
}

func (g *desktopOIDCGrants) redeem(code, proof string) (AuthResult, error) {
	if len(code) < 26 || len(code) > 128 || !validDesktopProof(proof) {
		return AuthResult{}, errors.New("invalid desktop SSO proof")
	}
	digest := sha256.Sum256([]byte(proof))
	challenge := base64.RawURLEncoding.EncodeToString(digest[:])
	g.mu.Lock()
	defer g.mu.Unlock()
	g.prune(time.Now())
	grant, ok := g.values[code]
	if !ok || subtle.ConstantTimeCompare([]byte(grant.challenge), []byte(challenge)) != 1 {
		return AuthResult{}, errors.New("desktop SSO grant is unavailable")
	}
	delete(g.values, code)
	return grant.result, nil
}

func (g *desktopOIDCGrants) session(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	// This exchange is for a native client. Browser requests use the existing
	// same-origin HttpOnly handoff and must not redeem desktop grants.
	if r.Header.Get("Origin") != "" || r.Header.Get("Sec-Fetch-Site") != "" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var input struct {
		Code  string `json:"code"`
		Proof string `json:"proof"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "invalid desktop SSO exchange", http.StatusBadRequest)
		return
	}
	result, err := g.redeem(input.Code, input.Proof)
	if err != nil {
		http.Error(w, "desktop SSO grant is unavailable", http.StatusUnauthorized)
		return
	}
	writeWorkspaceResponse(w, result, nil)
}
