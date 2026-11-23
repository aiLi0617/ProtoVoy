//go:build !js

package scheduler

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"protovoy.dev/protovoy/internal/project"
)

func TestRecordedExecutionOutcomes(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		execute Executor
		cancel  bool
		success bool
		status  int
	}{
		{"success", func(context.Context, project.Request) (Outcome, error) { return Outcome{true, 200}, nil }, false, true, 200},
		{"failure", func(context.Context, project.Request) (Outcome, error) {
			return Outcome{true, 502}, errors.New("secret upstream failure")
		}, false, false, 502},
		{"panic", func(context.Context, project.Request) (Outcome, error) { panic("secret panic payload") }, false, false, 0},
		{"timeout", func(ctx context.Context, _ project.Request) (Outcome, error) {
			<-ctx.Done()
			return Outcome{true, 200}, nil
		}, false, false, 200},
		{"shutdown", func(ctx context.Context, _ project.Request) (Outcome, error) {
			<-ctx.Done()
			return Outcome{}, ctx.Err()
		}, true, false, 0},
		{"invalid status", func(context.Context, project.Request) (Outcome, error) { return Outcome{true, 1000}, nil }, false, false, 0},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			s, err := Open(t.Context(), filepath.Join(t.TempDir(), "jobs.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			job, err := s.Create(t.Context(), "fixture", 10, true, project.Request{URL: "https://example.test", Method: "GET"}, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if scenario.cancel {
				cancel()
			}
			s.executeRecorded(ctx, job, 10*time.Millisecond, scenario.execute, func(err error) { t.Error(err) })
			results, err := s.Executions(t.Context())
			if err != nil || len(results) != 1 {
				t.Fatalf("results: %+v %v", results, err)
			}
			result := results[0]
			if result.Success != scenario.success || result.Status != scenario.status || result.JobID != job.ID || result.StartedAt.IsZero() {
				t.Fatalf("unexpected result: %+v", result)
			}
			jobs, err := s.List(t.Context())
			if err != nil || len(jobs) != 1 || jobs[0].LastExecutionID != result.ID {
				t.Fatalf("missing linked result: %+v %v", jobs, err)
			}
		})
	}
}
