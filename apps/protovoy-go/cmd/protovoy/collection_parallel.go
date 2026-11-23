package main

import (
	"context"
	"sync"

	"protovoy.dev/protovoy/internal/project"
)

type iterationOutcome struct {
	report []collectionResult
	failed int
	err    error
}

func runConcurrentIterations(ctx context.Context, value project.Project, options collectionRunOptions, emit func(collectionResult) error) (int, error) {
	ctx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() {
		cancel()
		workers.Wait()
	}()
	width := min(options.concurrency, len(options.rows))
	slots := make([]chan *iterationOutcome, width)
	launch := func(index int) {
		slot := make(chan *iterationOutcome, 1)
		slots[index%width] = slot
		workers.Go(func() {
			outcome := &iterationOutcome{}
			variables := iterationVariables(options.rows[index], value.CollectionVariables)
			outcome.failed, outcome.err = runCollectionIteration(ctx, value.Requests, variables,
				index+1, options, func(item collectionResult) error {
					outcome.report = append(outcome.report, item)
					return nil
				})
			slot <- outcome
		})
	}
	for index := range width {
		launch(index)
	}
	failed := 0
	for index := range options.rows {
		var outcome *iterationOutcome
		select {
		case <-ctx.Done():
			return failed, ctx.Err()
		case outcome = <-slots[index%width]:
		}
		for _, item := range outcome.report {
			if err := emit(item); err != nil {
				return failed, err
			}
		}
		failed += outcome.failed
		if outcome.err != nil {
			return failed, outcome.err
		}
		if next := index + width; next < len(options.rows) {
			launch(next)
		}
	}
	return failed, nil
}
