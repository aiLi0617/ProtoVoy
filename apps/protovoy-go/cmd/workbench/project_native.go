//go:build !js

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/vivaui/viva/app"
)

func chooseLocalFile(options app.OpenFileOptions, limit int, complete func([]byte, error)) {
	paths, err := app.OpenFiles(options)
	if err != nil {
		complete(nil, errors.New("could not open the file picker"))
		return
	}
	if len(paths) == 0 {
		complete(nil, nil)
		return
	}
	go func() {
		file, err := os.Open(paths[0])
		if err != nil {
			complete(nil, errors.New("could not open the selected file"))
			return
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
		switch {
		case err != nil:
			complete(nil, errors.New("could not read the selected file"))
		case len(data) > limit:
			complete(nil, fmt.Errorf("selected file exceeds %d KiB", limit>>10))
		default:
			complete(data, nil)
		}
	}()
}

func saveProjectFile(name string, data []byte) (bool, error) {
	return saveExportFile(name, "json", "JSON", "application/json", data)
}

func saveExportFile(name, extension, filter, _ string, data []byte) (bool, error) {
	path, err := app.SaveFile(app.SaveFileOptions{
		Title: "Export ProtoVoy " + filter, SuggestedName: name,
		DefaultExtension: extension, Filters: []app.FileFilter{{Name: filter, Pattern: "*." + extension}},
	})
	if err != nil {
		return false, errors.New("could not open the save dialog")
	}
	if path == "" {
		return false, nil
	}
	if err := replaceLocalFile(path, data); err != nil {
		return false, errors.New("could not save the export file")
	}
	return true, nil
}

func replaceLocalFile(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".protovoy-write-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
