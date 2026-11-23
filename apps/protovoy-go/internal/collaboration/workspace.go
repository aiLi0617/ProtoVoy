//go:build !js

package collaboration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"
)

type Workspace struct {
	ID        string          `json:"workspaceId"`
	Revision  int64           `json:"revision"`
	Document  json.RawMessage `json:"document"`
	UpdatedBy *string         `json:"updatedBy"`
	UpdatedAt *time.Time      `json:"updatedAt"`
}

type Conflict struct{ Current Workspace }

func (*Conflict) Error() string { return "workspace revision conflict" }

const maxWorkspacePayload = 8 << 20

var errInvalidWorkspaceUpdate = errors.New("invalid workspace update")

type Change struct {
	Revision int64           `json:"revision"`
	Patch    json.RawMessage `json:"patch"`
	ActorID  string          `json:"actorId"`
	Created  time.Time       `json:"createdAt"`
}

// WorkspaceStore persists revisions after the caller authorizes access to the organization.
type WorkspaceStore struct {
	db     *sql.DB
	events workspaceEvents
}

func NewWorkspaceStore(ctx context.Context, db *sql.DB) (*WorkspaceStore, error) {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS team_workspaces (
			organization_id TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
			revision INTEGER NOT NULL,
			document BLOB NOT NULL,
			updated_by TEXT NOT NULL,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY (organization_id, workspace_id)
		);
		CREATE TABLE IF NOT EXISTS team_workspace_changes (
			organization_id TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
			revision INTEGER NOT NULL,
			patch BLOB NOT NULL,
			actor_id TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			PRIMARY KEY (organization_id, workspace_id, revision)
		);
	`)
	if err != nil {
		return nil, err
	}
	if err := createAuditTable(ctx, db); err != nil {
		return nil, err
	}
	return &WorkspaceStore{db: db}, nil
}

func (store *WorkspaceStore) Get(ctx context.Context, organization, id string) (Workspace, error) {
	return readWorkspace(store.db.QueryRowContext(ctx, workspaceQuery, organization, id), id)
}

func (store *WorkspaceStore) Changes(ctx context.Context, organization, id string, after int64) ([]Change, error) {
	rows, err := store.db.QueryContext(ctx, `SELECT revision, patch, actor_id, created_at
		FROM team_workspace_changes WHERE organization_id = ? AND workspace_id = ? AND revision > ?
		ORDER BY revision`, organization, id, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	changes := []Change{}
	for rows.Next() {
		var change Change
		var stamp int64
		if err := rows.Scan(&change.Revision, &change.Patch, &change.ActorID, &stamp); err != nil {
			return nil, err
		}
		change.Created = time.UnixMilli(stamp).UTC()
		changes = append(changes, change)
	}
	return changes, rows.Err()
}

const workspaceQuery = `SELECT revision, document, updated_by, updated_at
	FROM team_workspaces WHERE organization_id = ? AND workspace_id = ?`

func readWorkspace(row *sql.Row, id string) (Workspace, error) {
	value := Workspace{ID: id}
	var actor string
	var updated int64
	err := row.Scan(&value.Revision, &value.Document, &actor, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		value.Document = json.RawMessage("{}")
		return value, nil
	}
	if err != nil {
		return Workspace{}, err
	}
	stamp := time.UnixMilli(updated).UTC()
	value.UpdatedBy, value.UpdatedAt = &actor, &stamp
	return value, nil
}

func (store *WorkspaceStore) Push(ctx context.Context, organization, id, actor string, base int64, document, patch json.RawMessage) (Workspace, error) {
	if organization == "" || id == "" || actor == "" || base < 0 ||
		len(document)+len(patch) > maxWorkspacePayload || !json.Valid(document) || !json.Valid(patch) {
		return Workspace{}, errInvalidWorkspaceUpdate
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return Workspace{}, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Truncate(time.Millisecond)
	row := tx.QueryRowContext(ctx, `
		INSERT INTO team_workspaces (organization_id, workspace_id, revision, document, updated_by, updated_at)
		SELECT ?, ?, 1, ?, ?, ? WHERE ? = 0 OR EXISTS (
			SELECT 1 FROM team_workspaces WHERE organization_id = ? AND workspace_id = ?
		)
		ON CONFLICT (organization_id, workspace_id) DO UPDATE SET
			revision = team_workspaces.revision + 1, document = excluded.document,
			updated_by = excluded.updated_by, updated_at = excluded.updated_at
		WHERE team_workspaces.revision = ?
		RETURNING revision
	`, organization, id, []byte(document), actor, now.UnixMilli(), base, organization, id, base)
	var revision int64
	if err := row.Scan(&revision); errors.Is(err, sql.ErrNoRows) {
		current, err := readWorkspace(tx.QueryRowContext(ctx, workspaceQuery, organization, id), id)
		if err != nil {
			return Workspace{}, err
		}
		return Workspace{}, &Conflict{Current: current}
	} else if err != nil {
		return Workspace{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO team_workspace_changes
		(organization_id, workspace_id, revision, patch, actor_id, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		organization, id, revision, []byte(patch), actor, now.UnixMilli()); err != nil {
		return Workspace{}, err
	}
	details := []byte(`{"revision":` + strconv.FormatInt(revision, 10) + "}")
	if err := recordAudit(ctx, tx, organization, actor, "workspace.sync", "workspace", id, details); err != nil {
		return Workspace{}, err
	}
	if err := tx.Commit(); err != nil {
		return Workspace{}, err
	}
	store.events.publish(organization, teamEvent{
		name: "workspace.changed", id: id + ":" + strconv.FormatInt(revision, 10),
		data: workspaceChanged{WorkspaceID: id, Revision: revision, ActorID: actor, At: now},
	})
	return Workspace{ID: id, Revision: revision, Document: document, UpdatedBy: &actor, UpdatedAt: &now}, nil
}
