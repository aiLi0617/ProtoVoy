//go:build !js

package collaboration

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"net/mail"
	"strings"
	"time"
)

var errFederatedIdentity = errors.New("federated identity is unavailable")

type federatedIdentity struct {
	issuer, subject, email, displayName string
	emailVerified                       bool
}

// loginFederated accepts claims only from the configured identity verifier.
func (store *AccessStore) loginFederated(ctx context.Context, identity federatedIdentity, organization, defaultRole string) (AuthResult, error) {
	switch defaultRole {
	case "VIEWER", "RUNNER", "EDITOR":
	default:
		return AuthResult{}, errFederatedIdentity
	}
	email := strings.ToLower(strings.TrimSpace(identity.email))
	address, err := mail.ParseAddress(email)
	if !identity.emailVerified || identity.issuer == "" || identity.subject == "" ||
		len(identity.issuer) > 2048 || len(identity.subject) > 255 || len(email) > 254 ||
		err != nil || address.Address != email {
		return AuthResult{}, errFederatedIdentity
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return AuthResult{}, err
	}
	defer tx.Rollback()
	var available bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM team_organizations WHERE id = ?)", organization).Scan(&available); err != nil {
		return AuthResult{}, err
	}
	if !available {
		return AuthResult{}, errFederatedIdentity
	}
	var user User
	err = tx.QueryRowContext(ctx, `SELECT a.id, a.email, a.display_name FROM team_identities i
		JOIN team_accounts a ON a.id = i.actor_id WHERE i.issuer = ? AND i.subject = ?`, identity.issuer, identity.subject).
		Scan(&user.ID, &user.Email, &user.DisplayName)
	if errors.Is(err, sql.ErrNoRows) {
		user, err = bindFederatedAccount(ctx, tx, identity, email)
	}
	if err != nil {
		return AuthResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO team_memberships (organization_id, actor_id, role)
		VALUES (?, ?, ?) ON CONFLICT (organization_id, actor_id) DO NOTHING`, organization, user.ID, defaultRole); err != nil {
		return AuthResult{}, err
	}
	var role string
	if err := tx.QueryRowContext(ctx, "SELECT role FROM team_memberships WHERE organization_id = ? AND actor_id = ?",
		organization, user.ID).Scan(&role); err != nil {
		return AuthResult{}, err
	}
	details, _ := json.Marshal(map[string]string{"provider": identity.issuer})
	if err := recordAudit(ctx, tx, organization, user.ID, "auth.sso", "federated_identity", identity.issuer, details); err != nil {
		return AuthResult{}, err
	}
	expires := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Millisecond)
	token, err := issueSession(ctx, tx, user.ID, expires)
	if err != nil {
		return AuthResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return AuthResult{}, err
	}
	return AuthResult{Token: token, ExpiresAt: expires, User: user, OrganizationID: organization, Role: role}, nil
}

func bindFederatedAccount(ctx context.Context, tx *sql.Tx, identity federatedIdentity, email string) (User, error) {
	user := User{Email: email}
	err := tx.QueryRowContext(ctx, "SELECT id, display_name FROM team_accounts WHERE email = ?", email).Scan(&user.ID, &user.DisplayName)
	if errors.Is(err, sql.ErrNoRows) {
		user.ID, user.DisplayName = rand.Text(), strings.TrimSpace(identity.displayName)
		if user.DisplayName == "" || len(user.DisplayName) > 200 {
			user.DisplayName = email
		}
		// Random hash material gives a federated-only account no usable local password.
		var salt [16]byte
		var hash [32]byte
		rand.Read(salt[:])
		rand.Read(hash[:])
		_, err = tx.ExecContext(ctx, `INSERT INTO team_accounts (id, email, display_name, password_salt, password_hash)
			VALUES (?, ?, ?, ?, ?)`, user.ID, email, user.DisplayName, salt[:], hash[:])
	}
	if err != nil {
		return User{}, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO team_identities (issuer, subject, actor_id)
		VALUES (?, ?, ?) ON CONFLICT DO NOTHING`, identity.issuer, identity.subject, user.ID)
	if err != nil {
		return User{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return User{}, err
	}
	if count == 0 {
		return User{}, errFederatedIdentity
	}
	return user, nil
}
