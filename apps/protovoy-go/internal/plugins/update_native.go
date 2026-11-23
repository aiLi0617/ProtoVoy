//go:build !js

package plugins

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
)

const activeSnapshotFile = ".active"
const snapshotPrefix = ".version-"

// UpdatePackage verifies a replacement before publishing its snapshot.
func UpdatePackage(store, source string, trust Trust) (Package, error) {
	next, err := ReadPackage(source, trust)
	if err != nil {
		return Package{}, err
	}
	return updateVerifiedPackage(store, next)
}

func UpdatePackageBytes(store, id string, manifestData, component []byte, trust Trust) (Package, error) {
	next, err := readPackageBytes(manifestData, component, trust)
	if err != nil {
		return Package{}, err
	}
	if next.Manifest.ID != id {
		return Package{}, errors.New("plugin update package does not match the selected ID")
	}
	return updateVerifiedPackage(store, next)
}

func updateVerifiedPackage(store string, next Package) (Package, error) {
	manifest, err := snapshotManifest(next)
	if err != nil {
		return Package{}, err
	}
	root, err := os.OpenRoot(store)
	if err != nil {
		return Package{}, errors.New("could not open plugin store")
	}
	defer root.Close()
	slot, err := openInstalledDirectory(root, next.Manifest.ID)
	if err != nil {
		return Package{}, err
	}
	defer slot.Close()
	data, err := readInstalledManifest(slot)
	if err != nil {
		return Package{}, err
	}
	previous, err := ReadManifest(data)
	if err != nil || previous.ID != next.Manifest.ID || previous.Kind != next.Manifest.Kind ||
		!samePublisher(previous.PublisherKeyID, next.Manifest.PublisherKeyID) {
		return Package{}, errors.New("plugin update must retain the installed identity, kind and publisher")
	}
	if previous.Version == next.Manifest.Version {
		return Package{}, errors.New("plugin update requires a different version")
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return Package{}, err
	}
	suffix := hex.EncodeToString(nonce[:])
	snapshot := snapshotPrefix + suffix
	if err := slot.Mkdir(snapshot, 0700); err != nil {
		return Package{}, errors.New("could not stage plugin update")
	}
	published := false
	defer func() {
		if !published {
			slot.RemoveAll(snapshot)
		}
	}()
	if err := writePackageSnapshot(slot, snapshot, suffix, manifest, next.Component); err != nil {
		return Package{}, err
	}
	pointer := ".activate-" + suffix
	defer slot.Remove(pointer)
	if err := writePluginFile(slot, pointer, []byte(snapshot)); err != nil {
		return Package{}, err
	}
	if err := slot.Rename(pointer, activeSnapshotFile); err != nil {
		return Package{}, errors.New("could not activate plugin update")
	}
	published = true
	return next, nil
}

func snapshotRevision(snapshot *os.Root) (string, error) {
	if _, err := snapshot.Lstat(".revision"); errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	data, err := readPackageFile(snapshot, ".revision", 32)
	if err != nil || len(data) != 32 {
		return "", errors.New("installed plugin snapshot revision is invalid")
	}
	if _, err := hex.DecodeString(string(data)); err != nil {
		return "", errors.New("installed plugin snapshot revision is invalid")
	}
	return string(data), nil
}

func samePublisher(previous, next *string) bool {
	if previous == nil || next == nil {
		return previous == nil && next == nil
	}
	return *previous == *next
}

func openActiveSnapshot(slot *os.Root) (*os.Root, error) {
	if _, err := slot.Lstat(activeSnapshotFile); errors.Is(err, os.ErrNotExist) {
		return slot.OpenRoot(".")
	}
	data, err := readPackageFile(slot, activeSnapshotFile, int64(len(snapshotPrefix)+32))
	name := string(data)
	if err != nil || len(name) != len(snapshotPrefix)+32 || name[:len(snapshotPrefix)] != snapshotPrefix {
		return nil, errors.New("installed plugin active snapshot is invalid")
	}
	if _, err := hex.DecodeString(name[len(snapshotPrefix):]); err != nil {
		return nil, errors.New("installed plugin active snapshot is invalid")
	}
	return openInstalledDirectory(slot, name)
}
