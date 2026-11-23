//go:build !js

package plugins

import (
	"errors"
	"os"
)

func packageDisabled(root *os.Root) (bool, error) {
	_, err := root.Lstat(".disabled")
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return true, errors.New("could not read plugin enablement state")
}

// SetEnabled changes a local execution policy, never publisher trust or grants.
// The marker's presence disables future executions; existing runs may finish.
func SetEnabled(store, id string, enabled bool) error {
	if !ValidID(id) {
		return errors.New("invalid plugin ID")
	}
	root, err := os.OpenRoot(store)
	if err != nil {
		return errors.New("could not open plugin store")
	}
	defer root.Close()
	child, err := openInstalledDirectory(root, id)
	if err != nil {
		return err
	}
	defer child.Close()
	data, err := readInstalledManifest(child)
	if err != nil {
		return err
	}
	manifest, err := ReadManifest(data)
	if err != nil || manifest.ID != id {
		return errors.New("installed plugin manifest is invalid or does not match its directory")
	}
	if enabled {
		if err := child.Remove(".disabled"); err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.New("could not enable plugin")
		}
		return nil
	}
	file, err := child.OpenFile(".disabled", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return errors.New("could not disable plugin")
	}
	syncErr, closeErr := file.Sync(), file.Close()
	if syncErr != nil || closeErr != nil {
		return errors.New("could not persist disabled plugin state")
	}
	return nil
}

func readInstalledManifest(slot *os.Root) ([]byte, error) {
	snapshot, err := openActiveSnapshot(slot)
	if err != nil {
		return nil, err
	}
	defer snapshot.Close()
	return readPackageFile(snapshot, "manifest.json", ManifestMaxBytes)
}

func readExecutablePackage(root *os.Root, trust Trust, expectedRevision string) (Package, error) {
	disabled, err := packageDisabled(root)
	if err != nil {
		return Package{}, err
	}
	if disabled {
		return Package{}, errors.New("plugin is disabled")
	}
	snapshot, err := openActiveSnapshot(root)
	if err != nil {
		return Package{}, err
	}
	defer snapshot.Close()
	if expectedRevision != "" {
		revision, err := snapshotRevision(snapshot)
		if err != nil || revision != expectedRevision {
			return Package{}, errors.New("installed plugin snapshot changed; refresh the plugin list and select it again")
		}
	}
	return readPackage(snapshot, trust)
}
