//go:build !js

package scheduler

import (
	"path/filepath"
	"testing"
	"time"

	"protovoy.dev/protovoy/internal/project"
)

func TestExecutionHistoryIsBoundedAndUpdatesJob(t *testing.T) {
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now()
	job, err := store.Create(t.Context(), "fixture", 10, true, project.Request{URL: "https://example.test", Method: "GET"}, now)
	if err != nil {
		t.Fatal(err)
	}
	var latest Execution
	for range 501 {
		latest, err = store.Record(t.Context(), job.ID, now, time.Millisecond, true, 204)
		if err != nil {
			t.Fatal(err)
		}
	}
	results, err := store.Executions(t.Context())
	if err != nil || len(results) != 500 || results[0].ID != latest.ID {
		t.Fatalf("unexpected retained history: %d, %v", len(results), err)
	}
	jobs, err := store.List(t.Context())
	if err != nil || jobs[0].LastExecutionID != latest.ID {
		t.Fatalf("latest execution not linked: %v", err)
	}
	if _, err := store.Delete(t.Context(), job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Record(t.Context(), job.ID, now, time.Millisecond, false, 0); err != nil {
		t.Fatal(err)
	}
	jobs, err = store.List(t.Context())
	if err != nil || len(jobs) != 0 {
		t.Fatal("late completion recreated deleted job")
	}
}
