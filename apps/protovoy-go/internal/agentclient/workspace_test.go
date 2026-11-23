package agentclient

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"protovoy.dev/protovoy/internal/project"
)

func TestWorkspaceClientUsesConditionalRequests(t *testing.T) {
	calls := 0
	client := &Client{origin: "http://127.0.0.1:39217", token: "fixture", do: func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/v1/workspace" || r.Header.Get("Authorization") != "Bearer fixture" {
			t.Fatal("workspace request lost Agent authentication")
		}
		switch calls {
		case 1:
			if r.Header.Get("If-None-Match") != "*" {
				t.Fatal("creation must require an absent workspace")
			}
		default:
			if r.Header.Get("If-Match") != "\"revision\"" {
				t.Fatal("update and delete must carry the read revision")
			}
		}
		return &http.Response{StatusCode: 204, Body: http.NoBody}, nil
	}}
	ctx := context.Background()
	value := project.Workspace{Version: 1}
	if err := client.CreateWorkspace(ctx, value); err != nil {
		t.Fatal(err)
	}
	if err := client.SaveWorkspace(ctx, value, "\"revision\""); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteWorkspace(ctx, "\"revision\""); err != nil {
		t.Fatal(err)
	}
	if err := client.SaveWorkspace(ctx, value, ""); err == nil || calls != 3 {
		t.Fatal("unversioned save reached the Agent")
	}
}

func TestWorkspaceClientRecognizesMissingAndChangedResources(t *testing.T) {
	for _, item := range []struct {
		status int
		want   error
	}{
		{http.StatusNotFound, ErrNotFound},
		{http.StatusPreconditionFailed, ErrConflict},
	} {
		client := &Client{origin: "http://127.0.0.1:39217", do: func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: item.status, Body: http.NoBody}, nil
		}}
		_, _, err := client.Workspace(context.Background())
		if !errors.Is(err, item.want) {
			t.Fatalf("HTTP %d returned %v, want %v", item.status, err, item.want)
		}
	}
}
