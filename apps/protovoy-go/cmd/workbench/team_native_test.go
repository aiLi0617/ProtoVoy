//go:build !js

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDesktopTeamLoginUsesEnteredOrigin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var credentials map[string]string
		if r.Method != "POST" || r.URL.Path != "/api/team/v1/auth/login" ||
			json.NewDecoder(r.Body).Decode(&credentials) != nil ||
			credentials["email"] != "owner@example.test" || credentials["password"] != "test-password" {
			t.Error("unexpected desktop login request")
			http.Error(w, "invalid login request", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"token":"test-token","organizationId":"team"}`))
	}))
	defer server.Close()
	workbench := newWorkbench()
	workbench.team.origin.SetText(server.URL)
	workbench.team.email.SetText("owner@example.test")
	workbench.team.password.SetText("test-password")
	workbench.loginTeam()
	if workbench.team.password.Text() != "" {
		t.Fatal("the password remained in the input after sign-in")
	}
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for workbench.cancel != nil {
		select {
		case <-timer.C:
			workbench.cancel()
			t.Fatal("desktop login did not finish")
		case <-ticker.C:
			workbench.finishTeamAction()
		}
	}
	if workbench.team.client == nil || workbench.team.client.Organization != "team" {
		t.Fatalf("desktop login did not create a session: %s", workbench.team.status)
	}
}
