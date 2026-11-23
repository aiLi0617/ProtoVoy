//go:build !js

package plugins

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
)

// RemoveInstalled detaches the ID before cleaning its private directory. Rooted
// operations confine cleanup to the store, including nested package symlinks.
// Broken manifests can be removed without parsing or executing guest content.
func RemoveInstalled(store, id string) (RemoveResult, error) {
	if !ValidID(id) {
		return RemoveResult{}, errors.New("invalid plugin ID")
	}
	root, err := os.OpenRoot(store)
	if err != nil {
		return RemoveResult{}, errors.New("could not open plugin store")
	}
	defer root.Close()
	child, err := openInstalledDirectory(root, id)
	if err != nil {
		return RemoveResult{}, err
	}
	child.Close()
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return RemoveResult{}, err
	}
	stage := ".remove-" + hex.EncodeToString(nonce[:])
	if err := root.Mkdir(stage, 0700); err != nil {
		return RemoveResult{}, errors.New("could not prepare plugin removal")
	}
	defer root.Remove(stage) // Only removes an empty container after failure.
	if err := root.Rename(id, filepath.Join(stage, "package")); err != nil {
		return RemoveResult{}, errors.New("could not detach installed plugin")
	}
	// stage is a generated relative directory created through this Root. Neither
	// client input nor filesystem symlinks can redirect recursive cleanup outside it.
	result := RemoveResult{ID: id}
	result.CleanupPending = root.RemoveAll(stage) != nil
	return result, nil
}
