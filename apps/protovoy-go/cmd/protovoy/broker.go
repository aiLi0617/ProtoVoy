package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"protovoy.dev/protovoy/internal/execution"
)

func readBrokerCA(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	pem, err := execution.ReadCAFile(path)
	if err == nil && len(pem) == 0 {
		err = errors.New("CA file is empty")
	}
	return string(pem), err
}

func runBroker(ctx context.Context, timeout time.Duration, output io.Writer, execute func(context.Context, func() error, func(any) error) error) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	err := execute(ctx, func() error { return nil }, json.NewEncoder(output).Encode)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
