package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"protovoy.dev/protovoy/internal/scheduler"
)

func validateScheduleStorage(path, teamPath, sqlTarget string) error {
	if path == ":memory:" || strings.HasPrefix(path, "file:") {
		return errors.New("-schedule-db requires a persistent filesystem path")
	}
	if err := checkTeamSQLIsolation(path, sqlTarget); err != nil {
		return errors.New("schedule storage must be separate from the SQL target and its SQLite sidecars")
	}
	if teamPath != "" && teamPath != ":memory:" {
		if strings.HasPrefix(teamPath, "file:") {
			return errors.New("use a filesystem path for -team-db when enabling schedules")
		}
		if err := checkTeamSQLIsolation(teamPath, "sqlite:"+url.PathEscape(filepath.ToSlash(path))+"?mode=rwc"); err != nil {
			return errors.New("schedule storage must be separate from the team database and its SQLite sidecars")
		}
	}
	return nil
}

// openScheduledService owns the runner and store as one lifecycle. The caller
// must close it before exiting, including when starting the HTTP server fails.
func openScheduledService(ctx context.Context, access webAccess, path string, timeout time.Duration, execute scheduler.Executor, reportError func(error)) (http.Handler, func(), error) {
	if execute == nil || timeout <= 0 || timeout > time.Hour {
		return nil, nil, errors.New("schedules require a protocol executor and a timeout within one hour")
	}
	store, err := scheduler.Open(ctx, path)
	if err != nil {
		return nil, nil, err
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := store.RunRecorded(runCtx, timeout, execute, reportError); err != nil && !errors.Is(err, context.Canceled) && reportError != nil {
			reportError(errors.New("schedule worker stopped"))
		}
	}()
	var once sync.Once
	closeService := func() {
		once.Do(func() {
			cancel()
			<-done
			_ = store.Close()
		})
	}
	api := store.Handler(func(r *http.Request) bool {
		return scheduleOriginAllowed(r, access.origin) && r.Header.Get("X-ProtoVoy-Scheduler") == "1"
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !access.authorize(w, r) {
			return
		}
		api.ServeHTTP(w, r)
	}), closeService, nil
}

func scheduleOriginAllowed(r *http.Request, origin string) bool {
	if sameOrigin(r, origin) {
		return true
	}
	// Fetch omits Origin on same-origin GET requests and forbids scripts from
	// supplying it. Accept browser-controlled fetch metadata for reads only;
	// mutations still require the exact Origin, and Host/login/marker checks
	// remain mandatory for both paths.
	return r.Method == http.MethodGet && len(r.Header.Values("Origin")) == 0 &&
		len(r.Header.Values("Sec-Fetch-Site")) == 1 && r.Header.Get("Sec-Fetch-Site") == "same-origin"
}
