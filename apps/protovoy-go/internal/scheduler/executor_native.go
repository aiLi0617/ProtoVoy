//go:build !js

package scheduler

import (
	"context"
	"errors"
	"time"

	"protovoy.dev/protovoy/internal/project"
)

// Outcome contains only the information safe to retain in execution history.
// Protocol adapters evaluate assertions before setting Success.
type Outcome struct {
	Success bool
	Status  int
}

type Executor func(context.Context, project.Request) (Outcome, error)

// RunRecorded applies a per-execution deadline and persists each outcome before
// releasing its worker slot. Executors must honor cancellation. reportError may
// be called concurrently and receives infrastructure errors only, never protocol
// response text or executor errors that could contain credentials.
func (s *Store) RunRecorded(ctx context.Context, timeout time.Duration, execute Executor, reportError func(error)) error {
	if execute == nil || timeout <= 0 || timeout > time.Hour {
		return errors.New("scheduler requires an executor and a timeout within one hour")
	}
	return s.Run(ctx, func(ctx context.Context, job Job) {
		s.executeRecorded(ctx, job, timeout, execute, reportError)
	}, reportError)
}

func (s *Store) executeRecorded(ctx context.Context, job Job, timeout time.Duration, execute Executor, reportError func(error)) {
	started := time.Now()
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var outcome Outcome
	var executionErr error
	func() {
		defer func() {
			if recover() != nil {
				outcome = Outcome{}
				executionErr = errors.New("scheduled executor panicked")
			}
		}()
		request, err := project.OpenHistoryRequest(job.Request)
		if err != nil {
			executionErr = err
			return
		}
		outcome, executionErr = execute(runCtx, request)
	}()
	if executionErr != nil || runCtx.Err() != nil {
		outcome.Success = false
	}
	if outcome.Status < 0 || outcome.Status > 999 {
		outcome.Status = 0
		outcome.Success = false
	}
	// Shutdown cancels the execution context, but the interrupted attempt still
	// needs a durable result. Keep this final write independently bounded.
	recordCtx, finish := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finish()
	if _, err := s.Record(recordCtx, job.ID, started, time.Since(started), outcome.Success, outcome.Status); err != nil && reportError != nil {
		reportError(errors.New("could not persist scheduled execution summary"))
	}
}
