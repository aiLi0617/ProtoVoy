//go:build !js

package collaboration

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver"
)

func TestWorkspaceStoreRevisionsAndOrganizationIsolation(t *testing.T) {
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "team.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := t.Context()
	store, err := NewWorkspaceStore(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	document, patch := json.RawMessage(`{"name":"jobs"}`), json.RawMessage(`[]`)
	if _, err := store.Push(ctx, "team", "workspace", "editor", 1, document, patch); err == nil {
		t.Fatal("created a workspace from a nonzero base revision")
	}
	for base := range int64(2) {
		value, err := store.Push(ctx, "team", "workspace", "editor", base, document, patch)
		if err != nil || value.Revision != base+1 {
			t.Fatalf("push from revision %d returned revision %d, error %v", base, value.Revision, err)
		}
	}
	var conflict *Conflict
	if _, err := store.Push(ctx, "team", "workspace", "editor", 1, document, patch); !errors.As(err, &conflict) ||
		conflict.Current.Revision != 2 || string(conflict.Current.Document) != string(document) {
		t.Fatalf("expected conflict with current snapshot, got %v", err)
	}
	changes, err := store.Changes(ctx, "team", "workspace", 1)
	if err != nil || len(changes) != 1 || changes[0].Revision != 2 || changes[0].ActorID != "editor" {
		t.Fatalf("unexpected changes: %+v, %v", changes, err)
	}
	other, err := store.Get(ctx, "other-team", "workspace")
	if err != nil || other.Revision != 0 || string(other.Document) != "{}" {
		t.Fatalf("unexpected workspace in another organization: %+v, %v", other, err)
	}
}

func TestWorkspaceRoutesRequireOrganizationAccessAndReturnConflicts(t *testing.T) {
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "team.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := NewWorkspaceStore(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	access, err := NewAccessStore(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	access.RegisterAuthRoutes(mux, "test-bootstrap-token")
	access.RegisterMemberRoutes(mux)
	access.RegisterAuditRoute(mux)
	if err := access.RegisterCommentRoutes(t.Context(), mux, store); err != nil {
		t.Fatal(err)
	}
	authCall := func(path, header, value, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("POST", "/v1/auth/"+path, strings.NewReader(body))
		if header != "" {
			r.Header.Set(header, value)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != status || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s returned %d, want %d: %s", path, w.Code, status, w.Body)
		}
		return w
	}
	const bootstrap = `{"email":"Owner@example.test","password":"test-owner-password","displayName":"Owner","organizationName":"Team"}`
	authCall("bootstrap", "X-ProtoVoy-Bootstrap-Token", "wrong", bootstrap, 403)
	created := authCall("bootstrap", "X-ProtoVoy-Bootstrap-Token", "test-bootstrap-token", bootstrap, 200)
	const password = "test-owner-password"
	var owner AuthResult
	if err := json.Unmarshal(created.Body.Bytes(), &owner); err != nil {
		t.Fatal(err)
	}
	if _, err := access.Bootstrap(t.Context(), "other@example.test", password, "Other", "Other"); !errors.Is(err, ErrAlreadyInitialized) {
		t.Fatalf("second bootstrap returned %v", err)
	}
	if _, err := access.Login(t.Context(), owner.User.Email, "incorrect"); !errors.Is(err, ErrInvalidLogin) {
		t.Fatalf("invalid login returned %v", err)
	}
	authCall("login", "", "", `{"email":"OWNER@example.test","password":"wrong"}`, 401)
	loggedIn := authCall("login", "", "", `{"email":"OWNER@example.test","password":"test-owner-password"}`, 200)
	var login AuthResult
	if err := json.Unmarshal(loggedIn.Body.Bytes(), &login); err != nil || login.Role != "OWNER" ||
		login.OrganizationID != owner.OrganizationID || login.Token == owner.Token {
		t.Fatal("login did not issue an independent owner session")
	}
	tokens := map[string]string{"editor": login.Token}
	store.RegisterWorkspaceRoutes(mux, access.WorkspaceAccess)
	path := "/v1/organizations/" + owner.OrganizationID + "/workspaces/jobs"
	const update = `{"baseRevision":0,"document":{"name":"jobs"}}`
	call := func(method, url, role, body string, status int) string {
		t.Helper()
		r := httptest.NewRequest(method, url, strings.NewReader(body))
		if token := tokens[role]; token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != status || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s %s returned %d, want %d: %s", method, url, w.Code, status, w.Body)
		}
		return w.Body.String()
	}
	members := "/v1/organizations/" + owner.OrganizationID + "/members"
	const provision = `{"email":"viewer@example.test","password":"test-viewer-password","displayName":"Viewer","role":"VIEWER"}`
	call("POST", members+"/provision", "", provision, 401)
	call("POST", members+"/provision", "editor", provision, 201)
	call("POST", members+"/provision", "editor", provision, 409)
	viewerResponse := authCall("login", "", "", `{"email":"viewer@example.test","password":"test-viewer-password"}`, 200)
	var viewer AuthResult
	if err := json.Unmarshal(viewerResponse.Body.Bytes(), &viewer); err != nil || viewer.Role != "VIEWER" {
		t.Fatal("provisioned viewer could not sign in")
	}
	tokens["viewer"] = viewer.Token
	call("POST", members+"/provision", "viewer", provision, 403)
	if result := call("GET", members, "viewer", "", 200); !strings.Contains(result, "viewer@example.test") {
		t.Fatalf("member list omitted the new viewer: %s", result)
	}
	call("GET", path, "", "", 401)
	call("PUT", path, "viewer", update, 403)
	call("PUT", path, "editor", update, 200)
	call("GET", "/v1/organizations/other/workspaces/jobs", "viewer", "", 403)
	result := call("PUT", path, "editor", update, 409)
	if !strings.Contains(result, `"currentRevision":1`) || !strings.Contains(result, `"currentDocument":{"name":"jobs"}`) {
		t.Fatalf("conflict omitted current workspace: %s", result)
	}
	call("GET", path+"/changes?afterRevision=invalid", "viewer", "", 400)
	if result := call("GET", path+"/changes?afterRevision=1", "viewer", "", 200); strings.TrimSpace(result) != "[]" {
		t.Fatalf("expected no changes past revision 1: %s", result)
	}
	const promoteViewer = `{"email":"viewer@example.test","role":"EDITOR"}`
	const nextUpdate = `{"baseRevision":1,"document":{"name":"jobs updated"}}`
	call("POST", members, "viewer", promoteViewer, 403)
	call("POST", members, "editor", `{"email":"owner@example.test","role":"VIEWER"}`, 409)
	call("POST", members, "editor", promoteViewer, 200)
	call("PUT", path, "viewer", nextUpdate, 200)
	call("POST", members, "editor", `{"email":"viewer@example.test","role":"VIEWER"}`, 200)
	call("PUT", path, "viewer", nextUpdate, 403)
	audit := "/v1/organizations/" + owner.OrganizationID + "/audit"
	call("GET", audit, "viewer", "", 403)
	call("GET", "/v1/organizations/other/audit", "editor", "", 403)
	body := call("GET", audit, "editor", "", 200)
	var events []AuditEvent
	if err := json.Unmarshal([]byte(body), &events); err != nil || len(events) != 6 ||
		events[0].Action != "member.upsert" || events[1].Action != "workspace.sync" ||
		string(events[1].Details) != `{"revision":2}` || events[1].ActorID != viewer.User.ID ||
		events[5].Action != "organization.bootstrap" || strings.Contains(body, password) || strings.Contains(body, viewer.Token) {
		t.Fatalf("unexpected organization audit events: %v", err)
	}
	comments := path + "/comments"
	var comment Comment
	body = call("POST", comments, "viewer", `{"body":"  Check this request  "}`, 201)
	if err := json.Unmarshal([]byte(body), &comment); err != nil || comment.ID == "" || comment.Body != "Check this request" ||
		comment.ActorID != viewer.User.ID || comment.ActorName != "Viewer" {
		t.Fatalf("unexpected comment creation: %v", err)
	}
	reply := `{"body":"Confirmed","parentId":"` + comment.ID + `"}`
	call("POST", strings.Replace(comments, "/jobs/", "/other/", 1), "viewer", reply, 404)
	call("POST", comments, "editor", reply, 201)
	commentURL := comments + "/" + comment.ID
	call("PATCH", commentURL, "viewer", `{"body":"Please check again"}`, 200)
	call("PATCH", commentURL+"/resolution", "viewer", `{"resolved":true}`, 403)
	body = call("PATCH", commentURL+"/resolution", "editor", `{"resolved":true}`, 200)
	if err := json.Unmarshal([]byte(body), &comment); err != nil || comment.ResolvedAt == nil || comment.ResolvedBy == nil {
		t.Fatalf("resolution was not recorded: %v", err)
	}
	body = call("PATCH", commentURL+"/resolution", "editor", `{"resolved":false}`, 200)
	if err := json.Unmarshal([]byte(body), &comment); err != nil || comment.ResolvedAt != nil || comment.ResolvedBy != nil {
		t.Fatalf("reopen did not clear resolution: %v", err)
	}
	body = call("GET", comments, "viewer", "", 200)
	var thread []Comment
	if err := json.Unmarshal([]byte(body), &thread); err != nil || len(thread) != 2 ||
		thread[0].Body != "Please check again" || thread[1].ParentID == nil || *thread[1].ParentID != thread[0].ID {
		t.Fatalf("unexpected thread: %v", err)
	}
	authCall("logout", "Authorization", "Bearer "+login.Token, "", 204)
	call("GET", path, "editor", "", 401)
	if _, err := db.Exec("UPDATE team_sessions SET expires_at = 0"); err != nil {
		t.Fatal(err)
	}
	call("GET", path, "viewer", "", 401)
}
