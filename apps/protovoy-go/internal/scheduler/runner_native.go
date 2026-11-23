//go:build !js

package scheduler

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Run polls persisted deadlines and waits for active executions when canceled.
// execute must honor its context. Each job has at most one active invocation
// within this runner.
func (s *Store) Run(ctx context.Context, execute func(context.Context, Job), reportError func(error)) error {
	if execute == nil {
		return errors.New("scheduler requires an executor")
	}
	var workers sync.WaitGroup
	defer workers.Wait()
	slots := make(chan struct{}, 4)
	finished := make(chan string, 4)
	active := map[string]bool{}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case id := <-finished:
			delete(active, id)
		case now := <-ticker.C:
			available := cap(slots) - len(slots)
			if available == 0 {
				continue
			}
			jobs, err := s.claimDue(ctx, now, available, active)
			if err != nil {
				if reportError != nil && ctx.Err() == nil {
					reportError(err)
				}
				continue
			}
			for _, job := range jobs {
				active[job.ID] = true
				slots <- struct{}{}
				workers.Go(func() {
					defer func() {
						<-slots
						select {
						case finished <- job.ID:
						case <-ctx.Done():
						}
					}()
					execute(ctx, job)
				})
			}
		}
	}
}
