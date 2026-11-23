//go:build !js

package collaboration

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestFederatedLoginBindsIssuerAndSubjectWithoutChangingRole(t *testing.T) {
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "team.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := t.Context()
	store, err := NewAccessStore(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.Bootstrap(ctx, "owner@example.test", "test-owner-password", "Owner", "Team")
	if err != nil {
		t.Fatal(err)
	}
	identity := federatedIdentity{issuer: "https://id.example.test", subject: "subject-1", email: "Member@example.test"}
	if _, err := store.loginFederated(ctx, identity, owner.OrganizationID, "VIEWER"); !errors.Is(err, errFederatedIdentity) {
		t.Fatalf("unverified email returned %v", err)
	}
	identity.emailVerified = true
	if _, err := store.loginFederated(ctx, identity, owner.OrganizationID, "ADMIN"); !errors.Is(err, errFederatedIdentity) {
		t.Fatalf("administrative default returned %v", err)
	}
	first, err := store.loginFederated(ctx, identity, owner.OrganizationID, "VIEWER")
	if err != nil || first.Role != "VIEWER" || first.Token == "" || first.User.Email != "member@example.test" {
		t.Fatalf("federated provisioning failed: %v", err)
	}
	other := identity
	other.subject = "subject-2"
	if _, err := store.loginFederated(ctx, other, owner.OrganizationID, "VIEWER"); !errors.Is(err, errFederatedIdentity) {
		t.Fatalf("a second subject for the same account returned %v", err)
	}
	identity.email = owner.User.Email
	second, err := store.loginFederated(ctx, identity, owner.OrganizationID, "EDITOR")
	if err != nil || second.User.ID != first.User.ID || second.Role != "VIEWER" || second.Token == first.Token {
		t.Fatalf("federated sign-in did not preserve the original account and role: %v", err)
	}
}
