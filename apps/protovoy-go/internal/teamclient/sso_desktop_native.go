//go:build !js

package teamclient

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

// CompleteDesktopSSO opens the existing hosted OIDC flow and exchanges its
// loopback code with a verifier that never leaves this client until redemption.
func (c *Client) CompleteDesktopSSO(parent context.Context, open func(string) error) error {
	if !strings.HasPrefix(c.base, "https://") || open == nil {
		return errors.New("desktop SSO requires an HTTPS team server and a browser opener")
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Minute)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	var config struct {
		Enabled bool `json:"enabled"`
		Desktop bool `json:"desktopEnabled"`
	}
	if err := c.request(ctx, http.MethodGet, "/auth/sso/config", nil, &config); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("could not check desktop SSO availability")
	}
	if !config.Enabled || !config.Desktop {
		return errors.New("desktop SSO is disabled or unsupported on this server")
	}
	var verifierBytes, stateBytes [32]byte
	if _, err := rand.Read(verifierBytes[:]); err != nil {
		return errors.New("could not prepare desktop SSO")
	}
	if _, err := rand.Read(stateBytes[:]); err != nil {
		return errors.New("could not prepare desktop SSO")
	}
	proof := base64.RawURLEncoding.EncodeToString(verifierBytes[:])
	state := base64.RawURLEncoding.EncodeToString(stateBytes[:])
	digest := sha256.Sum256([]byte(proof))
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return errors.New("could not open desktop SSO loopback callback")
	}
	defer listener.Close()
	codes := make(chan string, 1)
	server := &http.Server{ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, MaxHeaderBytes: 4096,
		Handler: desktopSSOCallback(listener.Addr().String(), state, codes)}
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
		_ = server.Close()
	}()
	stopped := make(chan error, 1)
	go func() { stopped <- server.Serve(listener) }()
	callback := "http://" + listener.Addr().String() + "/team-sso?" + url.Values{"state": {state}}.Encode()
	parameters := url.Values{"desktopReturn": {callback}, "desktopChallenge": {base64.RawURLEncoding.EncodeToString(digest[:])}}
	if err := ctx.Err(); err != nil {
		return err
	}
	opened := make(chan error, 1)
	go func() { opened <- open(c.base + "/auth/sso/login?" + parameters.Encode()) }()
	select {
	case err := <-opened:
		if err != nil {
			return errors.New("could not open the system browser for SSO")
		}
	case <-ctx.Done():
		return ctx.Err()
	case <-stopped:
		return errors.New("desktop SSO callback stopped")
	}
	var code string
	select {
	case code = <-codes:
	case <-ctx.Done():
		return ctx.Err()
	case <-stopped:
		return errors.New("desktop SSO callback stopped")
	}
	if err := c.authenticate(ctx, "/auth/sso/desktop/session", map[string]string{"code": code, "proof": proof}); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("desktop SSO session exchange failed")
	}
	return nil
}

func desktopSSOCallback(address, state string, codes chan<- string) http.Handler {
	var consumed atomic.Bool
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		ip, ipErr := netip.ParseAddr(host)
		if err != nil || ipErr != nil || !ip.IsLoopback() || r.Host != address || r.Header.Get("Origin") != "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/team-sso" || r.URL.RawPath != "" {
			http.NotFound(w, r)
			return
		}
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(r.RequestURI) > 2048 || len(query) != 2 || len(query["state"]) != 1 || len(query["code"]) != 1 ||
			subtle.ConstantTimeCompare([]byte(query.Get("state")), []byte(state)) != 1 || len(query.Get("code")) < 26 || len(query.Get("code")) > 128 {
			http.Error(w, "invalid desktop SSO callback", http.StatusBadRequest)
			return
		}
		if !consumed.CompareAndSwap(false, true) {
			http.Error(w, "authorization already received", http.StatusGone)
			return
		}
		select {
		case codes <- query.Get("code"):
			_, _ = w.Write([]byte("Authorization received. Return to ProtoVoy."))
		default:
			http.Error(w, "authorization already received", http.StatusGone)
		}
	})
}
