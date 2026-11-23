//go:build !js

package collaboration

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

func createAuditTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS team_audit (
			sequence INTEGER PRIMARY KEY,
			id TEXT NOT NULL UNIQUE,
			organization_id TEXT NOT NULL,
			actor_id TEXT NOT NULL,
			action TEXT NOT NULL,
			resource_type TEXT NOT NULL,
			resource_id TEXT NOT NULL,
			details BLOB NOT NULL,
			created_at INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS team_audit_organization ON team_audit (organization_id, sequence);
	`)
	return err
}

func recordAudit(ctx context.Context, tx *sql.Tx, org, actor, action, resourceType, resourceID string, details []byte) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO team_audit
		(id, organization_id, actor_id, action, resource_type, resource_id, details, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rand.Text(), org, actor, action, resourceType, resourceID, details, time.Now().UnixMilli())
	return err
}

type AuditEvent struct {
	ID           string          `json:"id"`
	ActorID      string          `json:"actorId"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resourceType"`
	ResourceID   string          `json:"resourceId"`
	Details      json.RawMessage `json:"details"`
	CreatedAt    time.Time       `json:"createdAt"`
}

func (store *AccessStore) RegisterAuditRoute(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/organizations/{organizationId}/audit", store.listAudit)
}

func (store *AccessStore) listAudit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	org := r.PathValue("organizationId")
	_, role, denial := store.organizationAccess(r, org)
	if denial == 0 && role != "OWNER" && role != "ADMIN" {
		denial = http.StatusForbidden
	}
	if denial != 0 {
		http.Error(w, http.StatusText(denial), denial)
		return
	}
	rows, err := store.db.QueryContext(r.Context(), `SELECT id, actor_id, action, resource_type, resource_id, details, created_at
		FROM team_audit WHERE organization_id = ? ORDER BY sequence DESC LIMIT 200`, org)
	if err != nil {
		writeWorkspaceResponse(w, nil, err)
		return
	}
	defer rows.Close()
	events := []AuditEvent{}
	for rows.Next() {
		var event AuditEvent
		var stamp int64
		if err := rows.Scan(&event.ID, &event.ActorID, &event.Action, &event.ResourceType, &event.ResourceID, &event.Details, &stamp); err != nil {
			writeWorkspaceResponse(w, nil, err)
			return
		}
		event.CreatedAt = time.UnixMilli(stamp).UTC()
		events = append(events, event)
	}
	writeWorkspaceResponse(w, events, rows.Err())
}
