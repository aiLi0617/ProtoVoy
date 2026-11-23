package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
	"protovoy.dev/protovoy/internal/secrets"
)

func TestOAuthBindingCommandAndCollectionExecution(t *testing.T) {
	keyring.MockInit()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, secret, ok := r.BasicAuth()
		if !ok || id != "client" || secret != "client-fixture" {
			t.Error("OAuth client credentials missing")
		}
		w.Write([]byte(`{"access_token":"issued-fixture"}`))
	}))
	defer provider.Close()
	resource := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer issued-fixture" {
			t.Error("OAuth bearer token missing")
		}
		w.WriteHeader(204)
	}))
	defer resource.Close()
	settings := execution.OAuthTokenRequest{Endpoint: provider.URL, GrantType: "client_credentials", ClientID: "client", ClientSecret: "client-fixture"}
	data, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := runSecretCommand([]string{"bind-oauth-http", "oauth-fixture", resource.URL}, bytes.NewReader(data), &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "client-fixture") {
		t.Fatal("credential printed")
	}
	request := project.Request{Name: "OAuth fixture", URL: resource.URL, Method: "GET", ExpectedHTTPStatus: 204, HTTPSecretReference: "oauth-fixture", HTTPSecretHeader: "Authorization"}
	if err := runCollection(t.Context(), project.Project{Requests: []project.Request{request}}, collectionRunOptions{}, io.Discard); err != nil {
		t.Fatal(err)
	}
	encoded, err := project.Encode(project.Project{Requests: []project.Request{request}})
	if err != nil || bytes.Contains(encoded, []byte("issued-fixture")) || bytes.Contains(encoded, []byte("client-fixture")) {
		t.Fatal("OAuth credentials persisted in project", err)
	}
	for _, input := range []string{string(data) + strings.Repeat(" ", 3000), string(data) + ` {}`, `{"unknown":"secret"}`} {
		if err := runSecretCommand([]string{"bind-oauth-http", "invalid-oauth", resource.URL}, strings.NewReader(input), io.Discard); err == nil {
			t.Fatal("invalid OAuth command input accepted")
		}
	}
}

func TestExportedCollectionUsesBoundCredential(t *testing.T) {
	keyring.MockInit()
	const credential = "Bearer fixture-value"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != credential {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	if err := runSecretCommand([]string{"bind-http", "fixture", server.URL, "Authorization"}, strings.NewReader(credential+"\n"), io.Discard); err != nil {
		t.Fatal(err)
	}
	value := project.Project{Name: "Fixture", Requests: []project.Request{{
		Name: "Authenticated", URL: server.URL, Method: "GET", ExpectedHTTPStatus: 204,
		HTTPSecretReference: "fixture", HTTPSecretHeader: "Authorization",
	}}}
	data, err := project.Encode(value)
	if err != nil || bytes.Contains(data, []byte(credential)) {
		t.Fatalf("invalid credential export: %v", err)
	}
	restored, err := project.Read(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := runCollection(t.Context(), restored, collectionRunOptions{}, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestProtocolPasswordSecretOverridesEnvironment(t *testing.T) {
	keyring.MockInit()
	t.Setenv("PROTOVOY_REDIS_PASSWORD", "environment-fixture")
	if err := secrets.Put("protocol-fixture", "keychain-fixture"); err != nil {
		t.Fatal(err)
	}
	if value, err := protocolPassword("redis", ""); err != nil || value != "environment-fixture" {
		t.Fatal("existing environment password was not preserved")
	}
	if value, err := protocolPassword("redis", "protocol-fixture"); err != nil || value != "keychain-fixture" {
		t.Fatal("explicit keychain reference did not override environment password")
	}
	if _, err := protocolPassword("http", "protocol-fixture"); err == nil {
		t.Fatal("unsupported protocol accepted password reference")
	}
}

func TestSecretHeaderResolution(t *testing.T) {
	keyring.MockInit()
	if err := secrets.Put("header-fixture", "Bearer fixture-private"); err != nil {
		t.Fatal(err)
	}
	values, err := addSecretHeaders([]string{"X-Test: yes"}, []string{"Authorization: header-fixture"})
	if err != nil {
		t.Fatal(err)
	}
	headers, err := parseCLIHeaders(values)
	if err != nil || len(headers) != 2 || headers[1][1] != "Bearer fixture-private" {
		t.Fatal("header reference did not resolve")
	}
	if err := secrets.Put("bad-header", "private\r\nInjected: yes"); err != nil {
		t.Fatal(err)
	}
	if _, err := addSecretHeaders(nil, []string{"Authorization: bad-header"}); err == nil || strings.Contains(err.Error(), "private") {
		t.Fatal("invalid header credential was accepted or exposed")
	}
}

func TestSecretCommandsDoNotPrintValues(t *testing.T) {
	keyring.MockInit()
	var output bytes.Buffer
	const value = "fixture-private-value"
	for _, action := range []string{"set", "exists", "delete"} {
		if err := runSecretCommand([]string{action, "fixture"}, strings.NewReader(value+"\r\n"), &output); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Contains(output.String(), value) {
		t.Fatal("secret command printed credential contents")
	}
	if err := runSecretCommand([]string{"exists", "fixture"}, strings.NewReader(""), &output); err == nil {
		t.Fatal("deleted secret still exists")
	}
}
