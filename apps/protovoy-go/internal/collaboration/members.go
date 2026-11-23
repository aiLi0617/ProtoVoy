//go:build !js

package collaboration

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

type Member struct {
	User
	Role string `json:"role"`
}

func (store *AccessStore) RegisterMemberRoutes(mux *http.ServeMux) {
	const path = "/v1/organizations/{organizationId}/members"
	mux.HandleFunc("GET "+path, store.listMembers)
	mux.HandleFunc("POST "+path, store.updateMember)
	mux.HandleFunc("POST "+path+"/provision", store.provisionMember)
}

func memberRoleStatus(actor, requested string) int {
	if actor != "OWNER" && actor != "ADMIN" {
		return http.StatusForbidden
	}
	switch requested {
	case "OWNER":
		if actor != "OWNER" {
			return http.StatusForbidden
		}
	case "ADMIN", "EDITOR", "RUNNER", "VIEWER":
	default:
		return http.StatusBadRequest
	}
	return 0
}

func (store *AccessStore) updateMember(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	org := r.PathValue("organizationId")
	var input struct{ Email, Role string }
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<10))
	if err != nil || json.Unmarshal(data, &input) != nil {
		http.Error(w, "invalid member details", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		writeWorkspaceResponse(w, nil, err)
		return
	}
	defer tx.Rollback()
	actor, actorRole, denial := organizationAccess(r, tx, org)
	if denial == 0 {
		denial = memberRoleStatus(actorRole, input.Role)
	}
	if denial != 0 {
		http.Error(w, http.StatusText(denial), denial)
		return
	}
	var member Member
	var previousRole sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT a.id, a.email, a.display_name, m.role
		FROM team_accounts a LEFT JOIN team_memberships m ON m.actor_id = a.id AND m.organization_id = ?
		WHERE a.email = ?`, org, strings.ToLower(strings.TrimSpace(input.Email))).
		Scan(&member.ID, &member.Email, &member.DisplayName, &previousRole)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	if err != nil {
		writeWorkspaceResponse(w, nil, err)
		return
	}
	if previousRole.String == "OWNER" && actorRole != "OWNER" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO team_memberships (organization_id, actor_id, role) VALUES (?, ?, ?)
		ON CONFLICT (organization_id, actor_id) DO UPDATE SET role = excluded.role
		WHERE team_memberships.role <> 'OWNER' OR excluded.role = 'OWNER' OR EXISTS (
			SELECT 1 FROM team_memberships WHERE organization_id = ? AND role = 'OWNER' AND actor_id <> ?
		)`, org, member.ID, input.Role, org, member.ID)
	if err != nil {
		writeWorkspaceResponse(w, nil, err)
		return
	}
	count, err := result.RowsAffected()
	if err != nil {
		writeWorkspaceResponse(w, nil, err)
		return
	}
	if count == 0 {
		http.Error(w, "an organization must retain an owner", http.StatusConflict)
		return
	}
	if err := recordAudit(ctx, tx, org, actor, "member.upsert", "user", member.ID, memberRoleDetails(input.Role)); err != nil {
		writeWorkspaceResponse(w, nil, err)
		return
	}
	if err := tx.Commit(); err != nil {
		writeWorkspaceResponse(w, nil, err)
		return
	}
	member.Role = input.Role
	writeWorkspaceResponse(w, member, nil)
}

func (store *AccessStore) listMembers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	org := r.PathValue("organizationId")
	if _, _, denial := store.organizationAccess(r, org); denial != 0 {
		http.Error(w, http.StatusText(denial), denial)
		return
	}
	rows, err := store.db.QueryContext(r.Context(), `
		SELECT a.id, a.email, a.display_name, m.role FROM team_memberships m
		JOIN team_accounts a ON a.id = m.actor_id WHERE m.organization_id = ?
		ORDER BY a.email`, org)
	if err != nil {
		writeWorkspaceResponse(w, nil, err)
		return
	}
	defer rows.Close()
	members := []Member{}
	for rows.Next() {
		var member Member
		if err := rows.Scan(&member.ID, &member.Email, &member.DisplayName, &member.Role); err != nil {
			writeWorkspaceResponse(w, nil, err)
			return
		}
		members = append(members, member)
	}
	writeWorkspaceResponse(w, members, rows.Err())
}

func (store *AccessStore) provisionMember(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	org := r.PathValue("organizationId")
	_, role, denial := store.organizationAccess(r, org)
	if denial != 0 {
		http.Error(w, http.StatusText(denial), denial)
		return
	}
	if role != "OWNER" && role != "ADMIN" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	var input struct {
		Email, Password, DisplayName, Role string
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<10))
	if err != nil || json.Unmarshal(data, &input) != nil {
		http.Error(w, "invalid member details", http.StatusBadRequest)
		return
	}
	if denial := memberRoleStatus(role, input.Role); denial != 0 {
		http.Error(w, http.StatusText(denial), denial)
		return
	}
	select {
	case store.passwordChecks <- struct{}{}:
		defer func() { <-store.passwordChecks }()
	default:
		http.Error(w, "account service is busy", http.StatusServiceUnavailable)
		return
	}
	user, salt, hash, err := prepareAccount(input.Email, input.Password, input.DisplayName)
	if err == ErrInvalidAccount {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err != nil {
		writeWorkspaceResponse(w, nil, err)
		return
	}
	ctx := r.Context()
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		writeWorkspaceResponse(w, nil, err)
		return
	}
	defer tx.Rollback()
	actor, role, denial := organizationAccess(r, tx, org)
	if denial == 0 {
		denial = memberRoleStatus(role, input.Role)
	}
	if denial != 0 {
		http.Error(w, http.StatusText(denial), denial)
		return
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO team_accounts (id, email, display_name, password_salt, password_hash)
		VALUES (?, ?, ?, ?, ?) ON CONFLICT(email) DO NOTHING`, user.ID, user.Email, user.DisplayName, salt, hash)
	if err != nil {
		writeWorkspaceResponse(w, nil, err)
		return
	}
	count, err := result.RowsAffected()
	if err != nil {
		writeWorkspaceResponse(w, nil, err)
		return
	}
	if count == 0 {
		http.Error(w, "user already exists", http.StatusConflict)
		return
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO team_memberships (organization_id, actor_id, role) VALUES (?, ?, ?)", org, user.ID, input.Role); err != nil {
		writeWorkspaceResponse(w, nil, err)
		return
	}
	if err := recordAudit(ctx, tx, org, actor, "member.provision", "user", user.ID, memberRoleDetails(input.Role)); err != nil {
		writeWorkspaceResponse(w, nil, err)
		return
	}
	if err := tx.Commit(); err != nil {
		writeWorkspaceResponse(w, nil, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(Member{User: user, Role: input.Role})
}

func memberRoleDetails(role string) []byte {
	details, _ := json.Marshal(map[string]string{"role": role})
	return details
}
