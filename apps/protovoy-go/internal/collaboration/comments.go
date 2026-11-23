//go:build !js

package collaboration

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type Comment struct {
	ID          string     `json:"id"`
	WorkspaceID string     `json:"workspaceId"`
	ActorID     string     `json:"actorId"`
	ActorName   string     `json:"actorName"`
	ParentID    *string    `json:"parentId"`
	Body        string     `json:"body"`
	ResolvedAt  *time.Time `json:"resolvedAt"`
	ResolvedBy  *string    `json:"resolvedBy"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

func (store *AccessStore) RegisterCommentRoutes(ctx context.Context, mux *http.ServeMux, workspaces *WorkspaceStore) error {
	_, err := store.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS team_comments (
			organization_id TEXT NOT NULL, workspace_id TEXT NOT NULL, id TEXT NOT NULL PRIMARY KEY,
			actor_id TEXT NOT NULL, parent_id TEXT, body TEXT NOT NULL,
			resolved_at INTEGER, resolved_by TEXT, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS team_comments_workspace
		ON team_comments (organization_id, workspace_id, created_at, id);
	`)
	if err != nil {
		return err
	}
	const path = "/v1/organizations/{organizationId}/workspaces/{workspaceId}/comments"
	mux.HandleFunc("GET "+path, store.listComments)
	mutate := func(w http.ResponseWriter, r *http.Request) {
		store.mutateComment(w, r, &workspaces.events)
	}
	mux.HandleFunc("POST "+path, mutate)
	mux.HandleFunc("PATCH "+path+"/{commentId}", mutate)
	mux.HandleFunc("PATCH "+path+"/{commentId}/resolution", func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("resolution", "true")
		mutate(w, r)
	})
	return nil
}

const commentQuery = `SELECT c.id, c.workspace_id, c.actor_id, a.display_name, c.parent_id, c.body,
	c.resolved_at, c.resolved_by, c.created_at, c.updated_at
	FROM team_comments c JOIN team_accounts a ON a.id = c.actor_id
	WHERE c.organization_id = ? AND c.workspace_id = ?`

func scanComment(row interface{ Scan(...any) error }) (Comment, error) {
	var comment Comment
	var created, updated int64
	var resolved sql.NullInt64
	err := row.Scan(&comment.ID, &comment.WorkspaceID, &comment.ActorID, &comment.ActorName, &comment.ParentID,
		&comment.Body, &resolved, &comment.ResolvedBy, &created, &updated)
	if err != nil {
		return Comment{}, err
	}
	comment.CreatedAt, comment.UpdatedAt = time.UnixMilli(created).UTC(), time.UnixMilli(updated).UTC()
	if resolved.Valid {
		stamp := time.UnixMilli(resolved.Int64).UTC()
		comment.ResolvedAt = &stamp
	}
	return comment, nil
}

func (store *AccessStore) listComments(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	org, workspace := r.PathValue("organizationId"), r.PathValue("workspaceId")
	if _, _, denial := store.organizationAccess(r, org); denial != 0 {
		http.Error(w, http.StatusText(denial), denial)
		return
	}
	rows, err := store.db.QueryContext(r.Context(), commentQuery+" ORDER BY c.created_at, c.rowid", org, workspace)
	if err != nil {
		writeWorkspaceResponse(w, nil, err)
		return
	}
	defer rows.Close()
	comments := []Comment{}
	for rows.Next() {
		comment, err := scanComment(rows)
		if err != nil {
			writeWorkspaceResponse(w, nil, err)
			return
		}
		comments = append(comments, comment)
	}
	writeWorkspaceResponse(w, comments, rows.Err())
}

func (store *AccessStore) mutateComment(w http.ResponseWriter, r *http.Request, events *workspaceEvents) {
	w.Header().Set("Cache-Control", "no-store")
	org, workspace, id := r.PathValue("organizationId"), r.PathValue("workspaceId"), r.PathValue("commentId")
	create, resolution := r.Method == http.MethodPost, r.PathValue("resolution") != ""
	var input struct {
		Body     string
		ParentID *string
		Resolved *bool
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32<<10))
	inputOK := err == nil && json.Unmarshal(data, &input) == nil
	body := strings.TrimSpace(input.Body)
	if !inputOK || (resolution && input.Resolved == nil) || (!resolution && (body == "" || utf8.RuneCountInString(body) > 4000)) {
		http.Error(w, "invalid comment update", http.StatusBadRequest)
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
	if denial != 0 {
		http.Error(w, http.StatusText(denial), denial)
		return
	}
	if resolution && role != "OWNER" && role != "ADMIN" && role != "EDITOR" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	now := time.Now().UnixMilli()
	action, details := "comment.edit", []byte("{}")
	if create {
		if input.ParentID != nil {
			var exists bool
			err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM team_comments
				WHERE organization_id = ? AND workspace_id = ? AND id = ?)`, org, workspace, *input.ParentID).Scan(&exists)
			if err != nil {
				writeWorkspaceResponse(w, nil, err)
				return
			}
			if !exists {
				http.Error(w, "parent comment not found in this workspace", http.StatusNotFound)
				return
			}
		}
		id = rand.Text()
		_, err = tx.ExecContext(ctx, `INSERT INTO team_comments
			(organization_id, workspace_id, id, actor_id, parent_id, body, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, org, workspace, id, actor, input.ParentID, body, now, now)
		action = "comment.create"
		details, _ = json.Marshal(map[string]string{"workspaceId": workspace})
	} else {
		var author string
		err := tx.QueryRowContext(ctx, "SELECT actor_id FROM team_comments WHERE organization_id = ? AND workspace_id = ? AND id = ?",
			org, workspace, id).Scan(&author)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "comment not found", http.StatusNotFound)
			return
		}
		if err != nil {
			writeWorkspaceResponse(w, nil, err)
			return
		}
		if resolution {
			var stamp, resolver any
			action = "comment.reopen"
			if *input.Resolved {
				stamp, resolver, action = now, actor, "comment.resolve"
			}
			_, err = tx.ExecContext(ctx, "UPDATE team_comments SET resolved_at = ?, resolved_by = ?, updated_at = ? WHERE id = ?", stamp, resolver, now, id)
			if err != nil {
				writeWorkspaceResponse(w, nil, err)
				return
			}
		} else {
			if author != actor && role != "OWNER" && role != "ADMIN" {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			if _, err := tx.ExecContext(ctx, "UPDATE team_comments SET body = ?, updated_at = ? WHERE id = ?", body, now, id); err != nil {
				writeWorkspaceResponse(w, nil, err)
				return
			}
		}
	}
	if err != nil {
		writeWorkspaceResponse(w, nil, err)
		return
	}
	comment, err := scanComment(tx.QueryRowContext(ctx, commentQuery+" AND c.id = ?", org, workspace, id))
	if err == nil {
		err = recordAudit(ctx, tx, org, actor, action, "workspace_comment", id, details)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeWorkspaceResponse(w, comment, err)
		return
	}
	eventAction := map[string]string{
		"comment.create": "created", "comment.edit": "edited", "comment.resolve": "resolved", "comment.reopen": "reopened",
	}[action]
	events.publish(org, teamEvent{
		name: "comment.changed", id: id + ":" + eventAction,
		data: commentChanged{WorkspaceID: workspace, CommentID: id, Action: eventAction, ActorID: actor, At: comment.UpdatedAt},
	})
	if !create {
		writeWorkspaceResponse(w, comment, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(comment)
}
