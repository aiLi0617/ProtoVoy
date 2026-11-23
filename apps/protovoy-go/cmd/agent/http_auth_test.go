package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
	"protovoy.dev/protovoy/internal/agentclient"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/secrets"
)

func TestAgentOAuthExchangeRunsInsideCancellableJob(t *testing.T) {
	for _, cancelExchange := range []bool{false, true} {
		name := "completed"
		if cancelExchange {
			name = "cancelled"
		}
		t.Run(name, func(t *testing.T) {
			keyring.MockInit()
			started := make(chan struct{})
			var resourceCalls atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				close(started)
				if cancelExchange {
					select {
					case <-r.Context().Done():
					case <-time.After(3 * time.Second):
					}
					return
				}
				_, _ = w.Write([]byte(`{"access_token":"agent-oauth-fixture"}`))
			}))
			defer provider.Close()
			resource := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				resourceCalls.Add(1)
				if r.Header.Get("Authorization") != "Bearer agent-oauth-fixture" {
					t.Error("OAuth token missing at resource")
				}
				_, _ = w.Write([]byte("OAuth resource fixture"))
			}))
			defer resource.Close()
			if err := secrets.PutHTTPOAuth("agent-oauth", resource.URL, execution.OAuthTokenRequest{
				Endpoint: provider.URL, GrantType: "client_credentials", ClientID: "client", ClientSecret: "fixture",
			}); err != nil {
				t.Fatal(err)
			}
			server := testAgent(t)
			client, err := agentclient.Pair(t.Context(), strings.TrimPrefix(server.URL, "http://"), pairingFixture, func(r *http.Request) (*http.Response, error) {
				r.Header.Set("Origin", browserOrigin)
				return http.DefaultClient.Do(r)
			})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				response, err := client.RunHTTPWithSecret(ctx, resource.URL, execution.HTTPPayload{Method: "GET"}, "agent-oauth", "Authorization", func(agentclient.Response) {})
				if err == nil && response.Preview != "OAuth resource fixture" {
					t.Error("OAuth resource response missing")
				}
				done <- err
			}()
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal("OAuth provider was not called")
			}
			if cancelExchange {
				cancel()
			}
			select {
			case err := <-done:
				if cancelExchange && err == nil || !cancelExchange && err != nil {
					t.Fatalf("unexpected OAuth outcome: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("Agent OAuth execution did not finish")
			}
			if cancelExchange && resourceCalls.Load() != 0 {
				t.Fatal("cancelled token exchange reached resource")
			}
		})
	}
}

func TestHTTPAuthReferenceThroughAgentClient(t *testing.T) {
	keyring.MockInit()
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer bound-fixture" {
			t.Error("bound credential did not reach upstream")
		}
		_, _ = w.Write([]byte("authenticated fixture"))
	}))
	defer upstream.Close()
	server := testAgent(t)
	do := func(r *http.Request) (*http.Response, error) {
		r.Header.Set("Origin", browserOrigin)
		return http.DefaultClient.Do(r)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	client, err := agentclient.Pair(ctx, strings.TrimPrefix(server.URL, "http://"), pairingFixture, do)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.PutHTTPSecret(ctx, "bound-fixture", upstream.URL, "Authorization", "Bearer bound-fixture"); err != nil {
		t.Fatal(err)
	}
	defer client.DeleteHTTPSecret(context.Background(), "bound-fixture")
	response, err := client.RunHTTPWithSecret(ctx, upstream.URL, execution.HTTPPayload{Method: "GET"}, "bound-fixture", "Authorization", func(agentclient.Response) {})
	if err != nil || response.Status != 200 || response.Preview != "authenticated fixture" {
		t.Fatalf("authenticated execution failed: status=%d err=%v", response.Status, err)
	}
	if _, err := client.RunHTTPWithSecret(ctx, upstream.URL+"/other", execution.HTTPPayload{Method: "GET"}, "bound-fixture", "Authorization", func(agentclient.Response) {}); err == nil {
		t.Fatal("different target accepted the credential")
	}
	if calls.Load() != 1 {
		t.Fatal("mismatched target reached upstream")
	}
}

func TestHTTPAuthReferenceBindsResolvedTarget(t *testing.T) {
	keyring.MockInit()
	const target = "https://example.test/private"
	if err := secrets.PutHTTP("fixture", target, "Authorization", "Bearer private-fixture"); err != nil {
		t.Fatal(err)
	}
	auth := json.RawMessage(`{"type":"header","secretRef":"fixture","header":"Authorization"}`)
	envelope := requestEnvelope{Protocol: "http", Target: "{{url}}", Variables: map[string]string{"url": target}, Auth: &auth}
	payload := httpEnvelopePayload{HTTPPayload: execution.HTTPPayload{Method: "GET"}}
	request, err := envelope.prepareHTTP(payload, time.Second)
	if err != nil || len(request.Headers) != 1 || request.Headers[0][1] != "Bearer private-fixture" {
		t.Fatal("bound credential was not applied to the resolved target")
	}
	envelope.Variables["url"] = "https://other.test/private"
	if _, err := envelope.prepareHTTP(payload, time.Second); err == nil {
		t.Fatal("credential resolved for another target")
	}
	envelope.Variables["url"] = target
	payload.FollowRedirects = true
	if _, err := envelope.prepareHTTP(payload, time.Second); err == nil {
		t.Fatal("credential request accepted redirects")
	}
}
