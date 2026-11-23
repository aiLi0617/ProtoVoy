package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"protovoy.dev/protovoy/internal/collaboration"
	"protovoy.dev/protovoy/internal/project"
	"protovoy.dev/protovoy/internal/teamclient"
)

func TestTeamHandlerRequiresHostedTransportAndAcceptsTeamAuth(t *testing.T) {
	access := webAccess{host: "workbench.example.test", origin: "https://workbench.example.test", auth: &webAuth{}}
	handler, closeTeam, err := openTeamHandler(t.Context(), access, filepath.Join(t.TempDir(), "team.db"), "test-bootstrap-token-long-enough", collaboration.OIDCConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer closeTeam()
	for _, tc := range []struct {
		url, origin string
		status      int
	}{
		{"http://workbench.example.test", "", 403},
		{"https://other.example.test", "", 403},
		{"https://workbench.example.test", "https://other.example.test", 403},
		{"https://workbench.example.test", access.origin, 200},
	} {
		r := httptest.NewRequest("GET", tc.url+"/api/team/v1/auth/bootstrap-status", nil)
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.status || (tc.status == 200 && !strings.Contains(w.Body.String(), `"available":true`)) {
			t.Fatalf("request for %s from %q returned %d: %s", tc.url, tc.origin, w.Code, w.Body)
		}
	}
}

func TestTeamClientSyncsThroughServer(t *testing.T) {
	const bootstrap = "test-bootstrap-token-long-enough"
	server := httptest.NewUnstartedServer(nil)
	access := loopbackAccess(server.Listener.Addr().String())
	dbPath := filepath.Join(t.TempDir(), "team.db")
	handler, closeTeam, err := openTeamHandler(t.Context(), access, dbPath, bootstrap, collaboration.OIDCConfig{})
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	defer closeTeam()
	server.Config.Handler = handler
	server.Start()
	defer server.Close()
	input := strings.NewReader(`{"email":"owner@example.test","password":"test-owner-password","displayName":"Owner","organizationName":"Team"}`)
	req, err := http.NewRequestWithContext(t.Context(), "POST", server.URL+"/api/team/v1/auth/bootstrap", input)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-ProtoVoy-Bootstrap-Token", bootstrap)
	res, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var owner struct{ Token, OrganizationID string }
	decodeErr := json.NewDecoder(res.Body).Decode(&owner)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("bootstrap returned %d", res.StatusCode)
	}
	if decodeErr != nil || owner.Token == "" {
		t.Fatalf("bootstrap response did not include a token: %v", decodeErr)
	}
	client, err := teamclient.New(server.URL, server.Client().Transport)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	if err := client.Login(ctx, "owner@example.test", "test-owner-password"); err != nil {
		t.Fatal(err)
	}
	streamContext, cancelStream := context.WithTimeout(ctx, 5*time.Second)
	defer cancelStream()
	eventReq, err := http.NewRequestWithContext(streamContext, "GET",
		server.URL+"/api/team/v1/organizations/"+owner.OrganizationID+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	eventReq.Header.Set("Authorization", "Bearer "+owner.Token)
	stream, err := server.Client().Do(eventReq)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	scanner := bufio.NewScanner(stream.Body)
	if stream.StatusCode != 200 || !scanner.Scan() || scanner.Text() != "event: connected" {
		t.Fatalf("event stream did not connect: status %d, error %v", stream.StatusCode, scanner.Err())
	}
	if _, revision, err := client.Load(ctx, "jobs"); err != nil || revision != 0 {
		t.Fatalf("initial load returned revision %d, %v", revision, err)
	}
	workspace := project.Project{
		Name: "Jobs", CollectionVariables: map[string]string{"host": "jobs.example.test"},
		Requests: []project.Request{{Name: "List jobs", Method: "GET", URL: "https://{{host}}/jobs", LocalID: "local-request"}},
	}
	if revision, err := client.Save(ctx, "jobs", 0, workspace); err != nil || revision != 1 {
		t.Fatalf("save returned revision %d, %v", revision, err)
	}
	var event string
	for scanner.Scan() {
		if line, ok := strings.CutPrefix(scanner.Text(), "data: "); ok && strings.Contains(line, `"workspaceId":"jobs"`) {
			event = line
			break
		}
	}
	if !strings.Contains(event, `"revision":1`) {
		t.Fatalf("workspace notification missing: %v", scanner.Err())
	}
	cancelStream()
	if saved, revision, err := client.Load(ctx, "jobs"); err != nil || revision != 1 || saved.Name != "Jobs" ||
		saved.CollectionVariables["host"] != "jobs.example.test" || len(saved.Requests) != 1 {
		t.Fatalf("load returned revision %d, %v", revision, err)
	}
	var conflict *teamclient.Conflict
	if _, err := client.Save(ctx, "jobs", 0, workspace); !errors.As(err, &conflict) || conflict.Revision != 1 {
		t.Fatalf("expected conflict at revision 1, got %v", err)
	}
	if strings.Contains(string(conflict.Document), "local-request") {
		t.Fatal("shared project contains a local request ID")
	}
	workspace.Requests[0].Headers = map[string]string{"Authorization": "fixture-private"}
	if _, err := client.Save(ctx, "jobs", 1, workspace); err == nil {
		t.Fatal("uploaded a project containing a literal authorization header")
	}
	if _, revision, err := client.Load(ctx, "jobs"); err != nil || revision != 1 {
		t.Fatalf("rejected upload changed the workspace: revision %d, %v", revision, err)
	}
	if err := client.Logout(ctx); err != nil {
		t.Fatal(err)
	}
	server.Close()
	if err := closeTeam(); err != nil {
		t.Fatal(err)
	}

	restarted := httptest.NewUnstartedServer(nil)
	access = loopbackAccess(restarted.Listener.Addr().String())
	handler, closeRestarted, err := openTeamHandler(ctx, access, dbPath, "", collaboration.OIDCConfig{})
	if err != nil {
		restarted.Close()
		t.Fatal(err)
	}
	defer closeRestarted()
	restarted.Config.Handler = handler
	restarted.Start()
	defer restarted.Close()
	client, err = teamclient.New(restarted.URL, restarted.Client().Transport)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Login(ctx, "owner@example.test", "test-owner-password"); err != nil {
		t.Fatalf("existing owner could not sign in after restart: %v", err)
	}
	if client.Organization != owner.OrganizationID {
		t.Fatal("organization changed after restart")
	}
	saved, revision, err := client.Load(ctx, "jobs")
	if err != nil || revision != 1 || saved.Name != "Jobs" ||
		saved.CollectionVariables["host"] != "jobs.example.test" || len(saved.Requests) != 1 || saved.Requests[0].Name != "List jobs" {
		t.Fatalf("published project was not restored after restart: revision %d, error %v", revision, err)
	}
}
