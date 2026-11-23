package agentclient

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestMockClientValidatesRulesBeforeSending(t *testing.T) {
	calls := 0
	client := &Client{origin: "http://127.0.0.1:39217", token: "session", do: func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/v1/mock" || r.Header.Get("Authorization") != "Bearer session" {
			t.Fatal("Mock management lost the authenticated Agent route")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"running":false,"address":""}`))}, nil
	}}
	ctx := context.Background()
	if _, err := client.StartMock(ctx, []byte("[")); err == nil || calls != 0 {
		t.Fatal("invalid rules reached Agent")
	}
	if _, err := client.StartMock(ctx, []byte("[]")); err != nil {
		t.Fatal(err)
	}
	if _, err := client.MockStatus(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.StopMock(ctx); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("got %d requests, want 3", calls)
	}
}

func TestMockCapabilityRequiresSupportedVersion(t *testing.T) {
	for _, version := range []string{"", "2", "unknown"} {
		client := &Client{mockVersion: version}
		if client.SupportsMock() {
			t.Fatalf("unsupported Mock API version %q was accepted", version)
		}
	}
	if !(&Client{mockVersion: "1"}).SupportsMock() {
		t.Fatal("supported Mock API version was rejected")
	}
}
