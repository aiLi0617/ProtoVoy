package main

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"protovoy.dev/protovoy/internal/project"
	"protovoy.dev/protovoy/internal/scheduler"
)

func TestScheduleStorageIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schedules.db")
	if err := validateScheduleStorage(path, "", ""); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		if err := validateScheduleStorage(path, path+suffix, ""); err == nil {
			t.Fatalf("team collision accepted: %s", suffix)
		}
		if err := validateScheduleStorage(path, "", "sqlite:"+filepath.ToSlash(path+suffix)+"?mode=rwc"); err == nil {
			t.Fatalf("SQL collision accepted: %s", suffix)
		}
	}
	for _, invalid := range []string{":memory:", "file:schedules.db?mode=memory"} {
		if err := validateScheduleStorage(invalid, "", ""); err == nil {
			t.Fatal("nonpersistent storage accepted")
		}
	}
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(filepath.Dir(path), "alias.db")
	if err := os.Link(path, alias); err == nil {
		if err := validateScheduleStorage(path, alias, ""); err == nil {
			t.Fatal("hard-linked team database accepted")
		}
	}
}

func TestScheduleOriginAllowsBrowserReadsOnly(t *testing.T) {
	for _, test := range []struct {
		method, origin, site string
		allowed              bool
	}{
		{"GET", "", "same-origin", true},
		{"GET", "", "cross-site", false},
		{"GET", "https://evil.test", "same-origin", false},
		{"GET", "", "", false},
		{"POST", "", "same-origin", false},
		{"PATCH", "", "same-origin", false},
		{"DELETE", "", "same-origin", false},
	} {
		r := httptest.NewRequest(test.method, "https://workbench.test/v1/jobs", nil)
		if test.origin != "" {
			r.Header.Set("Origin", test.origin)
		}
		if test.site != "" {
			r.Header.Set("Sec-Fetch-Site", test.site)
		}
		if actual := scheduleOriginAllowed(r, "https://workbench.test"); actual != test.allowed {
			t.Fatalf("%+v: %v", test, actual)
		}
	}
}

func TestScheduledServiceAuthorizesAndDrainsBeforeClosingStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schedules.db")
	store, err := scheduler.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Create(t.Context(), "due", 10, true, project.Request{Method: "GET", URL: "https://example.test"}, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	access := loopbackAccess("127.0.0.1:39300")
	started := make(chan struct{})
	handler, closeService, err := openScheduledService(t.Context(), access, path, time.Second, func(ctx context.Context, _ project.Request) (scheduler.Outcome, error) {
		close(started)
		<-ctx.Done()
		return scheduler.Outcome{}, ctx.Err()
	}, func(err error) { t.Error(err) })
	if err != nil {
		t.Fatal(err)
	}
	defer closeService()
	for _, scenario := range []struct {
		host, origin, marker string
		code                 int
	}{
		{access.host, access.origin, "1", 200},
		{"evil.test", access.origin, "1", 403},
		{access.host, "https://evil.test", "1", 403},
		{access.host, "", "1", 403},
		{access.host, access.origin, "", 403},
	} {
		r := httptest.NewRequest("GET", "http://"+scenario.host+"/v1/jobs", nil)
		r.Header.Set("Origin", scenario.origin)
		r.Header.Set("X-ProtoVoy-Scheduler", scenario.marker)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != scenario.code {
			t.Fatalf("%+v: %d", scenario, w.Code)
		}
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not start")
	}
	closeService()
	closeService() // callers can safely close after both startup failure and shutdown.
	store, err = scheduler.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	results, err := store.Executions(t.Context())
	if err != nil || len(results) != 1 || results[0].Success {
		t.Fatalf("interrupted result was lost: %+v %v", results, err)
	}
}
