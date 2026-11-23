//go:build !js

package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"protovoy.dev/protovoy/internal/project"
)

func workspacePath() (string, error) {
	dir := os.Getenv("PROTOVOY_CONFIG_DIR")
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(base, "protovoy")
	}
	return filepath.Join(dir, "protovoy-go-workspace.json"), nil
}

func readWorkspace() ([]byte, error) {
	path, err := workspacePath()
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, project.MaxBytes+1))
}

func writeWorkspace(data []byte) error {
	path, err := workspacePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return replaceLocalFile(path, data)
}

func onWorkspaceExit(func()) func() { return func() {} }
