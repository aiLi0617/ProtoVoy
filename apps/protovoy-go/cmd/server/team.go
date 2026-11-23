package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	_ "github.com/ncruces/go-sqlite3/driver"
	"protovoy.dev/protovoy/internal/collaboration"
	"protovoy.dev/protovoy/internal/execution"
)

const teamBootstrapEnv = "PROTOVOY_TEAM_BOOTSTRAP_TOKEN"

func checkTeamSQLIsolation(teamPath, target string) error {
	if teamPath == "" || teamPath == ":memory:" || !strings.HasPrefix(target, "sqlite:") || target == execution.SQLiteMemoryTarget {
		return nil
	}
	if strings.HasPrefix(teamPath, "file:") {
		return errors.New("use a filesystem path for -team-db when serving a file-backed SQLite target")
	}
	sqlPath, err := execution.SQLiteFilePath(target)
	if err != nil {
		return err
	}
	canonical := func(path string) (string, error) {
		path, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			return resolved, nil
		}
		if parent, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
			return filepath.Join(parent, filepath.Base(path)), nil
		}
		return path, nil
	}
	teamPath, err = canonical(teamPath)
	if err != nil {
		return err
	}
	sqlPath, err = canonical(sqlPath)
	if err != nil {
		return err
	}
	// Compare sidecars in both directions, including files SQLite has not
	// created yet. A target named team.db-wal would otherwise pass startup.
	suffixes := []string{"", "-wal", "-shm", "-journal"}
	for _, teamSuffix := range suffixes {
		teamFile := teamPath + teamSuffix
		teamInfo, teamErr := os.Stat(teamFile)
		for _, sqlSuffix := range suffixes {
			sqlFile := sqlPath + sqlSuffix
			sqlInfo, sqlErr := os.Stat(sqlFile)
			if teamFile == sqlFile || runtime.GOOS == "windows" && strings.EqualFold(teamFile, sqlFile) ||
				teamErr == nil && sqlErr == nil && os.SameFile(teamInfo, sqlInfo) {
				return errors.New("SQL target and its SQLite sidecars must be separate from the team database and its sidecars")
			}
		}
	}
	return nil
}

func openTeamHandler(ctx context.Context, access webAccess, path, bootstrapToken string, oidcConfig collaboration.OIDCConfig) (http.Handler, func() error, error) {
	if bootstrapToken != "" && (!printableCredential(bootstrapToken) || len(bootstrapToken) < 24) {
		return nil, nil, errors.New(teamBootstrapEnv + " requires at least 24 printable characters")
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, nil, err
	}
	db.SetMaxOpenConns(1)
	accounts, err := collaboration.NewAccessStore(ctx, db)
	if err != nil {
		db.Close()
		return nil, nil, err
	}
	workspaces, err := collaboration.NewWorkspaceStore(ctx, db)
	if err != nil {
		db.Close()
		return nil, nil, err
	}
	mux := http.NewServeMux()
	accounts.RegisterAuthRoutes(mux, bootstrapToken)
	if err := accounts.RegisterOIDCRoutes(ctx, mux, access.origin, oidcConfig); err != nil {
		db.Close()
		return nil, nil, err
	}
	accounts.RegisterMemberRoutes(mux)
	accounts.RegisterAuditRoute(mux)
	if err := accounts.RegisterCommentRoutes(ctx, mux, workspaces); err != nil {
		db.Close()
		return nil, nil, err
	}
	workspaces.RegisterWorkspaceRoutes(mux, accounts.WorkspaceAccess)
	workspaces.RegisterEventRoute(ctx, mux, accounts.WorkspaceAccess)
	routes := http.StripPrefix("/api/team", mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Host != access.host || (access.auth != nil && r.TLS == nil) ||
			(len(r.Header.Values("Origin")) != 0 && !sameOrigin(r, access.origin)) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		routes.ServeHTTP(w, r)
	}), db.Close, nil
}
