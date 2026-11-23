//go:build !js

package plugins

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
)

// Inventory reads only bounded manifests through the store's rooted handle.
// Missing stores are empty; malformed packages remain visible with an error.
func Inventory(store string) ([]Installed, error) {
	result := []Installed{}
	root, err := os.OpenRoot(store)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return nil, errors.New("could not open plugin store")
	}
	defer root.Close()
	directory, err := root.Open(".")
	if err != nil {
		return nil, errors.New("could not read plugin store")
	}
	defer directory.Close()
	entries, err := directory.ReadDir(1025)
	if err != nil && err != io.EOF || len(entries) > 1024 {
		return nil, errors.New("plugin store could not be read within its 1024 entry limit")
	}
	encodedSize := 3 // brackets and final newline
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		item := Installed{ID: entry.Name()}
		child, err := openInstalledDirectory(root, entry.Name())
		if err != nil {
			item.Error = "could not open installed plugin"
		} else {
			var data []byte
			snapshot, readErr := openActiveSnapshot(child)
			if readErr == nil {
				item.Revision, readErr = snapshotRevision(snapshot)
				if readErr == nil {
					data, readErr = readPackageFile(snapshot, "manifest.json", ManifestMaxBytes)
				}
				snapshot.Close()
			}
			disabled, stateErr := packageDisabled(child)
			item.Disabled = disabled
			child.Close()
			manifest, manifestErr := ReadManifest(data)
			if readErr != nil || manifestErr != nil || manifest.ID != item.ID {
				item.Error = "installed plugin manifest is invalid or does not match its directory"
			} else {
				item.Manifest = &manifest
				if stateErr != nil {
					item.Error = stateErr.Error()
				}
			}
		}
		encoded, err := json.Marshal(item)
		if err != nil || encodedSize+len(encoded)+1 > InventoryMaxBytes {
			return nil, errors.New("plugin inventory exceeds its 1 MiB response limit")
		}
		encodedSize += len(encoded) + 1
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}
