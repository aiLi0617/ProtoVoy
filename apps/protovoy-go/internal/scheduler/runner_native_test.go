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

func TestRunnerCancelsAndWaitsForExecution(t *testing.T) {
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.Create(t.Context(), "due", 10, true,
		project.Request{URL: "https://example.test", Method: "GET"}, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started, stopped := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- store.Run(ctx, func(ctx context.Context, job Job) {
			close(started)
			<-ctx.Done()
			close(stopped)
		}, func(err error) { t.Error(err) })
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("due job was not executed")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		select {
		case <-stopped:
		default:
			t.Fatal("runner returned before execution stopped")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runner did not stop")
	}
}
