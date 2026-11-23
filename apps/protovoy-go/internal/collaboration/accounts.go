//go:build !js

package collaboration

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"errors"
	"net/mail"
	"strings"
	"time"
)

var (
	ErrAlreadyInitialized = errors.New("team collaboration is already initialized")
	ErrInvalidAccount     = errors.New("invalid team account details")
	ErrInvalidLogin       = errors.New("invalid email or password")
	ErrNoMembership       = errors.New("no organization membership")
)

type User struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
}

type AuthResult struct {
	Token          string    `json:"token"`
	ExpiresAt      time.Time `json:"expiresAt"`
	User           User      `json:"user"`
	OrganizationID string    `json:"organizationId"`
	Role           string    `json:"role"`
}

func (store *AccessStore) Initialized(ctx context.Context) (bool, error) {
	var initialized bool
	err := store.db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM team_accounts)").Scan(&initialized)
	return initialized, err
}

// Bootstrap creates the first organization. The caller must first verify the bootstrap secret.
func (store *AccessStore) Bootstrap(ctx context.Context, email, password, displayName, organization string) (AuthResult, error) {
	organization = strings.TrimSpace(organization)
	if organization == "" || len(organization) > 200 {
		return AuthResult{}, ErrInvalidAccount
	}
	user, salt, hash, err := prepareAccount(email, password, displayName)
	if err != nil {
		return AuthResult{}, err
	}
	org := rand.Text()
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return AuthResult{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO team_accounts (id, email, display_name, password_salt, password_hash)
		SELECT ?, ?, ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM team_accounts)`, user.ID, user.Email, user.DisplayName, salt, hash)
	if err != nil {
		return AuthResult{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return AuthResult{}, err
	}
	if count == 0 {
		return AuthResult{}, ErrAlreadyInitialized
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO team_organizations (id, name) VALUES (?, ?)", org, organization); err != nil {
		return AuthResult{}, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO team_memberships (organization_id, actor_id, role) VALUES (?, ?, 'OWNER')", org, user.ID); err != nil {
		return AuthResult{}, err
	}
	if err := recordAudit(ctx, tx, org, user.ID, "organization.bootstrap", "organization", org, []byte("{}")); err != nil {
		return AuthResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return AuthResult{}, err
	}
	return store.authResult(ctx, user, org, "OWNER")
}

func prepareAccount(email, password, displayName string) (User, []byte, []byte, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	displayName = strings.TrimSpace(displayName)
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 254 || len(password) < 10 || len(password) > 200 ||
		displayName == "" || len(displayName) > 200 {
		return User{}, nil, nil, ErrInvalidAccount
	}
	var salt [16]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return User{}, nil, nil, err
	}
	hash, err := teamPasswordHash(password, salt[:])
	if err != nil {
		return User{}, nil, nil, err
	}
	user := User{ID: rand.Text(), Email: email, DisplayName: displayName}
	return user, salt[:], hash, nil
}

func (store *AccessStore) Login(ctx context.Context, email, password string) (AuthResult, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if len(email) > 254 || len(password) > 200 || password == "" {
		return AuthResult{}, ErrInvalidLogin
	}
	user := User{Email: email}
	salt, expected := make([]byte, 16), make([]byte, 32)
	err := store.db.QueryRowContext(ctx, "SELECT id, display_name, password_salt, password_hash FROM team_accounts WHERE email = ?", email).
		Scan(&user.ID, &user.DisplayName, &salt, &expected)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return AuthResult{}, err
	}
	hash, hashErr := teamPasswordHash(password, salt)
	if hashErr != nil {
		return AuthResult{}, hashErr
	}
	if subtle.ConstantTimeCompare(hash, expected) != 1 || err != nil {
		return AuthResult{}, ErrInvalidLogin
	}
	var org, role string
	err = store.db.QueryRowContext(ctx, `SELECT organization_id, role FROM team_memberships
		WHERE actor_id = ? ORDER BY organization_id LIMIT 1`, user.ID).Scan(&org, &role)
	if errors.Is(err, sql.ErrNoRows) {
		return AuthResult{}, ErrNoMembership
	}
	if err != nil {
		return AuthResult{}, err
	}
	return store.authResult(ctx, user, org, role)
}

func (store *AccessStore) authResult(ctx context.Context, user User, organization, role string) (AuthResult, error) {
	expires := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Millisecond)
	token, err := store.IssueSession(ctx, user.ID, expires)
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{Token: token, ExpiresAt: expires, User: user, OrganizationID: organization, Role: role}, nil
}

func teamPasswordHash(password string, salt []byte) ([]byte, error) {
	return pbkdf2.Key(sha256.New, password, salt, 600_000, 32)
}
