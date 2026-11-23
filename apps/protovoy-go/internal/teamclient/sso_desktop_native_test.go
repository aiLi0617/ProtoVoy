//go:build !js

package teamclient

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDesktopSSOLoopbackAndProofExchangeCreateSession(t *testing.T) {
	const code = "abcdefghijklmnopqrstuvwxyz"
	var challenge atomic.Value
	var exchanges atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/team/v1/auth/sso/config":
			_, _ = io.WriteString(w, `{"enabled":true,"desktopEnabled":true}`)
		case "/api/team/v1/auth/sso/desktop/session":
			var input struct{ Code, Proof string }
			if r.Method != "POST" || r.Header.Get("Origin") != "" || json.NewDecoder(r.Body).Decode(&input) != nil || input.Code != code {
				t.Error("unexpected native exchange")
				http.Error(w, "invalid", 400)
				return
			}
			digest := sha256.Sum256([]byte(input.Proof))
			if base64.RawURLEncoding.EncodeToString(digest[:]) != challenge.Load().(string) {
				t.Error("native proof did not match browser challenge")
				http.Error(w, "invalid", 400)
				return
			}
			exchanges.Add(1)
			_, _ = io.WriteString(w, `{"token":"fixture-session","organizationId":"team","user":{"id":"user"},"role":"VIEWER"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := New(server.URL, server.Client().Transport)
	if err != nil {
		t.Fatal(err)
	}
	callbackURL := ""
	err = client.CompleteDesktopSSO(t.Context(), func(target string) error {
		u, err := url.Parse(target)
		if err != nil || u.Scheme != "https" || u.Host != strings.TrimPrefix(server.URL, "https://") || u.Path != "/api/team/v1/auth/sso/login" {
			return errors.New("unexpected browser URL")
		}
		challenge.Store(u.Query().Get("desktopChallenge"))
		callbackURL = u.Query().Get("desktopReturn")
		callback, err := url.Parse(callbackURL)
		if err != nil || callback.Hostname() != "127.0.0.1" || callback.Path != "/team-sso" || len(callback.Query().Get("state")) != 43 {
			return errors.New("unsafe native callback")
		}
		response, err := http.Get(callbackURL + "&code=" + code)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK || response.Header.Get("Cache-Control") != "no-store" {
			return errors.New("callback was rejected or cacheable")
		}
		body, err := io.ReadAll(response.Body)
		if err != nil || strings.Contains(string(body), "fixture-session") {
			return errors.New("callback exposed session data")
		}
		return nil
	})
	if err != nil || exchanges.Load() != 1 || client.token != "fixture-session" || client.Organization != "team" || client.UserID != "user" || client.Role != "VIEWER" {
		t.Fatalf("desktop sign-in failed: %v", err)
	}
	assertDesktopCallbackClosed(t, callbackURL)
}

func assertDesktopCallbackClosed(t *testing.T, target string) {
	t.Helper()
	u, err := url.Parse(target)
	if err != nil || u.Host == "" {
		t.Fatal("missing callback URL")
	}
	connection, err := net.DialTimeout("tcp", u.Host, time.Second)
	if err == nil {
		connection.Close()
		t.Fatal("native callback listener remained open")
	}
}

func TestDesktopSSOCancelAndBrowserFailureCloseListener(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"enabled":true,"desktopEnabled":true}`)
	}))
	defer server.Close()
	for _, cancelFlow := range []bool{true, false} {
		client, err := New(server.URL, server.Client().Transport)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		callbackURL := ""
		err = client.CompleteDesktopSSO(ctx, func(target string) error {
			u, _ := url.Parse(target)
			callbackURL = u.Query().Get("desktopReturn")
			if cancelFlow {
				cancel()
				return nil
			}
			return errors.New("private-browser-failure-details")
		})
		cancel()
		if err == nil || cancelFlow && !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "private-browser-failure-details") || client.token != "" {
			t.Fatal("cancelled or failed browser launch created a session or leaked details")
		}
		assertDesktopCallbackClosed(t, callbackURL)
	}
	client, _ := New("http://127.0.0.1:39300", nil)
	if err := client.CompleteDesktopSSO(t.Context(), func(string) error { t.Fatal("opened browser for plaintext team server"); return nil }); err == nil {
		t.Fatal("accepted plaintext desktop SSO")
	}
	client, _ = New(server.URL, server.Client().Transport)
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	var deadlineCallback atomic.Value
	err := client.CompleteDesktopSSO(ctx, func(target string) error {
		u, _ := url.Parse(target)
		deadlineCallback.Store(u.Query().Get("desktopReturn"))
		return nil
	})
	if !errors.Is(err, context.DeadlineExceeded) || client.token != "" {
		t.Fatal("desktop SSO did not honor the caller's timeout")
	}
	if callback, ok := deadlineCallback.Load().(string); ok {
		assertDesktopCallbackClosed(t, callback)
	}
}

func TestDesktopSSOCallbackRejectsRebindingStateMismatchAndReplay(t *testing.T) {
	const address = "127.0.0.1:43821"
	state := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	codes := make(chan string, 1)
	handler := desktopSSOCallback(address, state, codes)
	valid := "http://" + address + "/team-sso?state=" + state + "&code=abcdefghijklmnopqrstuvwxyz"
	for _, test := range []struct {
		method, target, host, remote, origin string
		status                               int
	}{
		{"GET", valid, "attacker.example:43821", "127.0.0.1:45000", "", 403},
		{"GET", valid, address, "192.0.2.1:45000", "", 403},
		{"GET", valid, address, "127.0.0.1:45000", "https://attacker.example", 403},
		{"POST", valid, address, "127.0.0.1:45000", "", 405},
		{"GET", strings.Replace(valid, "/team-sso?", "/other?", 1), address, "127.0.0.1:45000", "", 404},
		{"GET", strings.Replace(valid, state, "wrong", 1), address, "127.0.0.1:45000", "", 400},
		{"GET", valid + "&state=" + state, address, "127.0.0.1:45000", "", 400},
	} {
		request := httptest.NewRequest(test.method, test.target, nil)
		request.Host = test.host
		request.RemoteAddr = test.remote
		if test.origin != "" {
			request.Header.Set("Origin", test.origin)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.status || len(codes) != 0 || response.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatalf("unsafe callback status %d, want %d", response.Code, test.status)
		}
	}
	for _, status := range []int{200, 410} {
		request := httptest.NewRequest("GET", valid, nil)
		request.RemoteAddr = "127.0.0.1:45000"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != status {
			t.Fatalf("callback returned %d, want %d", response.Code, status)
		}
		if status == 200 {
			<-codes
		}
	}
}

func TestDesktopSSOCancellationDoesNotWaitForBlockedBrowserLauncher(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"enabled":true,"desktopEnabled":true}`)
	}))
	defer server.Close()
	client, _ := New(server.URL, server.Client().Transport)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	callback := make(chan string, 1)
	release := make(chan struct{})
	defer close(release)
	done := make(chan error, 1)
	go func() {
		done <- client.CompleteDesktopSSO(ctx, func(target string) error {
			u, _ := url.Parse(target)
			callback <- u.Query().Get("desktopReturn")
			<-release
			return nil
		})
	}()
	var uri string
	select {
	case uri = <-callback:
	case <-time.After(5 * time.Second):
		t.Fatal("browser launcher did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("cancel did not finish blocked browser launch")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel waited for the browser launcher")
	}
	assertDesktopCallbackClosed(t, uri)
}
