//go:build !js

package collaboration

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"
)

type AccessStore struct {
	db             *sql.DB
	passwordChecks chan struct{}
}

func NewAccessStore(ctx context.Context, db *sql.DB) (*AccessStore, error) {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS team_accounts (
			id TEXT NOT NULL PRIMARY KEY,
			email TEXT NOT NULL UNIQUE,
			display_name TEXT NOT NULL,
			password_salt BLOB NOT NULL,
			password_hash BLOB NOT NULL
		);
		CREATE TABLE IF NOT EXISTS team_organizations (
			id TEXT NOT NULL PRIMARY KEY,
			name TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS team_identities (
			issuer TEXT NOT NULL,
			subject TEXT NOT NULL,
			actor_id TEXT NOT NULL,
			PRIMARY KEY (issuer, subject),
			UNIQUE (issuer, actor_id)
		);
		CREATE TABLE IF NOT EXISTS team_memberships (
			organization_id TEXT NOT NULL,
			actor_id TEXT NOT NULL,
			role TEXT NOT NULL CHECK (role IN ('OWNER','ADMIN','EDITOR','RUNNER','VIEWER')),
			PRIMARY KEY (organization_id, actor_id)
		);
		CREATE TABLE IF NOT EXISTS team_sessions (
			token_hash BLOB NOT NULL PRIMARY KEY,
			actor_id TEXT NOT NULL,
			expires_at INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS team_sessions_expiration ON team_sessions (expires_at);
	`)
	if err != nil {
		return nil, err
	}
	if err := createAuditTable(ctx, db); err != nil {
		return nil, err
	}
	return &AccessStore{db: db, passwordChecks: make(chan struct{}, 4)}, nil
}

// IssueSession must be called only after the account's identity is verified.
func (store *AccessStore) IssueSession(ctx context.Context, actor string, expires time.Time) (string, error) {
	return issueSession(ctx, store.db, actor, expires)
}

func issueSession(ctx context.Context, db interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, actor string, expires time.Time) (string, error) {
	if actor == "" || !expires.After(time.Now()) {
		return "", errors.New("invalid team session")
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(secret[:])
	hash := sha256.Sum256([]byte(token))
	_, err := db.ExecContext(ctx, `INSERT INTO team_sessions (token_hash, actor_id, expires_at)
		VALUES (?, ?, ?)`, hash[:], actor, expires.UnixMilli())
	if err != nil {
		return "", err
	}
	return token, nil
}

func (store *AccessStore) RevokeSession(ctx context.Context, token string) error {
	hash := sha256.Sum256([]byte(token))
	_, err := store.db.ExecContext(ctx, "DELETE FROM team_sessions WHERE token_hash = ?", hash[:])
	return err
}

func teamBearerToken(r *http.Request) (string, bool) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return token, ok && len(token) == base64.RawURLEncoding.EncodedLen(32)
}

func (store *AccessStore) WorkspaceAccess(r *http.Request, organization string, edit bool) (string, int) {
	actor, role, denial := store.organizationAccess(r, organization)
	if denial != 0 {
		return "", denial
	}
	if edit && role != "OWNER" && role != "ADMIN" && role != "EDITOR" {
		return "", http.StatusForbidden
	}
	return actor, 0
}

func (store *AccessStore) organizationAccess(r *http.Request, organization string) (string, string, int) {
	return organizationAccess(r, store.db, organization)
}

func organizationAccess(r *http.Request, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, organization string) (string, string, int) {
	token, ok := teamBearerToken(r)
	if !ok {
		return "", "", http.StatusUnauthorized
	}
	hash := sha256.Sum256([]byte(token))
	var actor string
	var role sql.NullString
	err := db.QueryRowContext(r.Context(), `
		SELECT s.actor_id, m.role FROM team_sessions s
		LEFT JOIN team_memberships m ON m.actor_id = s.actor_id AND m.organization_id = ?
		WHERE s.token_hash = ? AND s.expires_at > ?
	`, organization, hash[:], time.Now().UnixMilli()).Scan(&actor, &role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", http.StatusUnauthorized
	}
	if err != nil {
		return "", "", http.StatusInternalServerError
	}
	if !role.Valid {
		return "", "", http.StatusForbidden
	}
	return actor, role.String, 0
}
