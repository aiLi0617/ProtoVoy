package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/scripting"
)

func runScriptedHTTP(ctx context.Context, request execution.HTTPRequest, prePath, postPath string, output, status io.Writer) error {
	if prePath == "" && postPath == "" {
		return run(ctx, request, output, status)
	}
	pre, err := readHTTPScript(prePath)
	if err != nil {
		return fmt.Errorf("pre-request script: %w", err)
	}
	post, err := readHTTPScript(postPath)
	if err != nil {
		return fmt.Errorf("post-response script: %w", err)
	}
	var preScripts, postScripts []string
	if prePath != "" {
		preScripts = []string{pre}
	}
	if postPath != "" {
		postScripts = []string{post}
	}
	input, data, variables, err := scripting.PrepareHTTP(ctx, request, preScripts, nil)
	if err != nil {
		return err
	}
	if len(postScripts) == 0 {
		return run(ctx, input, output, status)
	}
	code, body, err := scripting.ExecuteHTTPPost(ctx, execution.HTTPClient{}, input, postScripts, data, variables)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(status, "HTTP %d\n", code); err != nil {
		return err
	}
	_, err = output.Write(body)
	return err
}

func readHTTPScript(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	file, err := os.Open(path)
	if err != nil {
		return "", errors.New("could not open script file")
	}
	defer file.Close()
	const limit = 64 << 10
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || len(data) > limit {
		return "", errors.New("could not read script file within 64 KiB")
	}
	return string(data), nil
}
