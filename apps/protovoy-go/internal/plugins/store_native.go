//go:build !js

package plugins

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

func openInstalledDirectory(root *os.Root, id string) (*os.Root, error) {
	info, err := root.Lstat(id)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("installed plugin is not a directory inside the store")
	}
	child, err := root.OpenRoot(id)
	if err != nil {
		return nil, errors.New("could not open installed plugin inside the store")
	}
	opened, err := child.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		child.Close()
		return nil, errors.New("installed plugin directory changed while opening it")
	}
	return child, nil
}

func StoreDirectory(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return "", errors.New("could not find user configuration directory; set PROTOVOY_PLUGIN_DIR")
	}
	return filepath.Join(config, "ProtoVoy", "plugins"), nil
}

// InstallPackage publishes a verified snapshot. It never overwrites an installed
// package and never executes guest code during installation.
func InstallPackage(store, source string, trust Trust) (Package, error) {
	pack, err := ReadPackage(source, trust)
	if err != nil {
		return Package{}, err
	}
	return installVerifiedPackage(store, pack)
}

// InstallPackageBytes accepts an uploaded pair without accessing client paths.
func InstallPackageBytes(store string, manifestData, component []byte, trust Trust) (Package, error) {
	pack, err := readPackageBytes(manifestData, component, trust)
	if err != nil {
		return Package{}, err
	}
	return installVerifiedPackage(store, pack)
}

func readPackageBytes(manifestData, component []byte, trust Trust) (Package, error) {
	manifest, err := readTrustedManifest(manifestData, trust)
	if err != nil {
		return Package{}, err
	}
	return verifyPackage(manifest, component, trust)
}

func installVerifiedPackage(store string, pack Package) (Package, error) {
	manifest, err := snapshotManifest(pack)
	if err != nil {
		return Package{}, err
	}
	if err := os.MkdirAll(store, 0700); err != nil {
		return Package{}, errors.New("could not create plugin store")
	}
	root, err := os.OpenRoot(store)
	if err != nil {
		return Package{}, errors.New("could not open plugin store")
	}
	defer root.Close()
	if _, err := root.Lstat(pack.Manifest.ID); !errors.Is(err, os.ErrNotExist) {
		return Package{}, errors.New("plugin is already installed or its destination is unavailable")
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return Package{}, err
	}
	revision := hex.EncodeToString(nonce[:])
	stage := ".install-" + revision
	if err := root.Mkdir(stage, 0700); err != nil {
		return Package{}, errors.New("could not stage plugin installation")
	}
	// The generated relative path is resolved by Root; cleanup cannot escape the store.
	defer root.RemoveAll(stage)
	if err := writePackageSnapshot(root, stage, revision, manifest, pack.Component); err != nil {
		return Package{}, err
	}
	// A competing valid installation has a nonempty directory, which Rename
	// cannot replace on supported platforms. Readers see only the complete pair.
	if err := root.Rename(stage, pack.Manifest.ID); err != nil {
		return Package{}, errors.New("could not publish plugin installation; destination may already exist")
	}
	return pack, nil
}

func snapshotManifest(pack Package) ([]byte, error) {
	manifest, err := json.Marshal(pack.Manifest)
	if err != nil {
		return nil, err
	}
	if len(manifest) > ManifestMaxBytes {
		return nil, errors.New("encoded plugin manifest exceeds 64 KiB")
	}
	return manifest, nil
}

func writePackageSnapshot(root *os.Root, stage, revision string, manifest, component []byte) error {
	for name, data := range map[string][]byte{"manifest.json": manifest, "plugin.wasm": component, ".revision": []byte(revision)} {
		if err := writePluginFile(root, filepath.Join(stage, name), data); err != nil {
			return err
		}
	}
	return nil
}

func writePluginFile(root *os.Root, name string, data []byte) error {
	file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return errors.New("could not create installed plugin file")
	}
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return errors.New("could not persist installed plugin file")
	}
	return nil
}
