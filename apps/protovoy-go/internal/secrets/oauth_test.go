//go:build !js

package secrets

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
	"protovoy.dev/protovoy/internal/execution"
)

func TestOAuthBindingChecksResourceBeforeExchangingToken(t *testing.T) {
	keyring.MockInit()
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		id, secret, ok := r.BasicAuth()
		if !ok || id != "client" || secret != "fixture" {
			t.Error("client credentials missing")
		}
		fmt.Fprint(w, `{"access_token":"issued-token"}`)
	}))
	defer provider.Close()
	const target = "https://resource.test/api"
	if err := PutHTTPOAuth("oauth", target, execution.OAuthTokenRequest{Endpoint: provider.URL, GrantType: "client_credentials", ClientID: "client", ClientSecret: "fixture"}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("saving a binding performed network access")
	}
	for _, destination := range []struct{ target, header string }{{target + "/other", "Authorization"}, {target, "X-Api-Key"}} {
		if _, err := ResolveHTTPContext(t.Context(), "oauth", destination.target, destination.header); err == nil {
			t.Fatal("mismatched resource binding accepted")
		}
	}
	if _, err := ResolveHTTP("oauth", target, "Authorization"); err == nil {
		t.Fatal("context-free OAuth exchange accepted")
	}
	if calls.Load() != 0 {
		t.Fatal("rejected binding reached provider")
	}
	headers, err := HTTPHeadersContext(t.Context(), nil, target, "oauth", "Authorization", false)
	if err != nil || len(headers) != 1 || headers[0][1] != "Bearer issued-token" {
		t.Fatalf("injection: %v %v", headers, err)
	}
	if calls.Load() != 1 {
		t.Fatal("token was not exchanged exactly once")
	}
}

func TestOAuthBindingPropagatesCancellation(t *testing.T) {
	keyring.MockInit()
	started := make(chan struct{})
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		close(started)
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer provider.Close()
	const target = "https://resource.test/api"
	if err := PutHTTPOAuth("oauth", target, execution.OAuthTokenRequest{Endpoint: provider.URL, GrantType: "client_credentials", ClientID: "client", ClientSecret: "fixture"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := ResolveHTTPContext(ctx, "oauth", target, "Authorization"); done <- err }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("exchange did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("cancellation: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("exchange did not cancel")
	}
}
