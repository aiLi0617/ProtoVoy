//go:build !js

package scheduler

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"protovoy.dev/protovoy/internal/project"
)

func TestScheduleEnablementResetsDeadlineOnlyOnTransition(t *testing.T) {
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	job, err := store.Create(t.Context(), "fixture", 10, true,
		project.Request{URL: "https://example.test", Method: "GET"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetEnabled(t.Context(), job.ID, false, now); err != nil {
		t.Fatal(err)
	}
	later := now.Add(time.Hour)
	if jobs, err := store.ClaimDue(t.Context(), later, 4); err != nil || len(jobs) != 0 {
		t.Fatalf("disabled job was claimed: %+v, %v", jobs, err)
	}
	enabled, err := store.SetEnabled(t.Context(), job.ID, true, later)
	if err != nil || !enabled.NextRunAt.Equal(later.Add(10*time.Second)) {
		t.Fatalf("unexpected enable deadline: %+v, %v", enabled, err)
	}
	repeated, err := store.SetEnabled(t.Context(), job.ID, true, later.Add(time.Second))
	if err != nil || !repeated.NextRunAt.Equal(enabled.NextRunAt) {
		t.Fatal("repeated enable changed deadline")
	}
}

func TestActiveScheduleKeepsItsDeadlineUntilItCanRun(t *testing.T) {
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	job, err := store.Create(t.Context(), "active", 10, true,
		project.Request{URL: "https://example.test", Method: "GET"}, now)
	if err != nil {
		t.Fatal(err)
	}
	dueAt := now.Add(20 * time.Second)
	jobs, err := store.claimDue(t.Context(), dueAt, 4, map[string]bool{job.ID: true})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("active schedule was claimed: %+v, %v", jobs, err)
	}
	persisted, err := store.List(t.Context())
	if err != nil || len(persisted) != 1 || !persisted[0].NextRunAt.Equal(job.NextRunAt) {
		t.Fatalf("active deadline changed: %+v, %v", persisted, err)
	}
	jobs, err = store.ClaimDue(t.Context(), dueAt, 4)
	if err != nil || len(jobs) != 1 || !jobs[0].NextRunAt.Equal(dueAt.Add(10*time.Second)) {
		t.Fatalf("released schedule was not claimed: %+v, %v", jobs, err)
	}
}

func TestConcurrentClaimsDoNotDuplicateDueJob(t *testing.T) {
	ctx := t.Context()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "schedules.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	request := project.Request{URL: "https://example.test", Method: "GET"}
	if _, err := store.Create(ctx, "enabled", 10, true, request, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(ctx, "disabled", 10, false, request, now); err != nil {
		t.Fatal(err)
	}
	results := make(chan []Job, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Go(func() {
			jobs, err := store.ClaimDue(ctx, now.Add(10*time.Second), 16)
			if err != nil {
				t.Error(err)
			}
			results <- jobs
		})
	}
	workers.Wait()
	close(results)
	count := 0
	for jobs := range results {
		count += len(jobs)
	}
	if count != 1 {
		t.Fatalf("claimed %d jobs, expected one", count)
	}
}

func TestJobsSurviveRestartAndRejectCredentials(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "schedules.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	request := project.Request{URL: "https://example.test", Method: "GET"}
	now := time.Now().UTC()
	job, err := store.Create(ctx, "fixture", 10, true, request, now)
	if err != nil {
		t.Fatal(err)
	}
	request.Headers = map[string]string{"Authorization": "fixture"}
	if _, err := store.Create(ctx, "credential", 10, true, request, now); err == nil {
		t.Fatal("persisted credentials")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	jobs, err := store.List(ctx)
	if err != nil || len(jobs) != 1 || jobs[0].ID != job.ID || !jobs[0].NextRunAt.Equal(now.Add(10*time.Second)) {
		t.Fatalf("unexpected persisted jobs: %+v, %v", jobs, err)
	}
	if deleted, err := store.Delete(ctx, job.ID); err != nil || !deleted {
		t.Fatalf("delete: %v, %v", deleted, err)
	}
	if deleted, err := store.Delete(ctx, job.ID); err != nil || deleted {
		t.Fatalf("repeated delete: %v, %v", deleted, err)
	}
}
