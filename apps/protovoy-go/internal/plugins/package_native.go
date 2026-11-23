//go:build !js

package plugins

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
)

type Package struct {
	Manifest          Manifest
	Component         []byte
	SHA256            string
	SignatureVerified bool
}

type Trust struct {
	Keys          map[string]ed25519.PublicKey
	Permissions   map[string]bool
	AllowUnsigned bool
}

// ReadPackage reads through a rooted directory so package symlinks cannot escape
// to unrelated host files. Component validation belongs to the execution runtime.
func ReadPackage(directory string, trust Trust) (Package, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return Package{}, errors.New("could not open plugin package")
	}
	defer root.Close()
	snapshot, err := openActiveSnapshot(root)
	if err != nil {
		return Package{}, err
	}
	defer snapshot.Close()
	return readPackage(snapshot, trust)
}

func readPackage(root *os.Root, trust Trust) (Package, error) {
	data, err := readPackageFile(root, "manifest.json", ManifestMaxBytes)
	if err != nil {
		return Package{}, err
	}
	manifest, err := readTrustedManifest(data, trust)
	if err != nil {
		return Package{}, err
	}
	component, err := readPackageFile(root, "plugin.wasm", ComponentMaxBytes)
	if err != nil {
		return Package{}, err
	}
	return verifyPackage(manifest, component, trust)
}

func readTrustedManifest(data []byte, trust Trust) (Manifest, error) {
	manifest, err := ReadManifest(data)
	if err != nil {
		return Manifest{}, err
	}
	for _, permission := range manifest.Permissions {
		if !trust.Permissions[permission] {
			return Manifest{}, errors.New("plugin permission was not granted: " + permission)
		}
	}
	return manifest, nil
}

func verifyPackage(manifest Manifest, component []byte, trust Trust) (Package, error) {
	if len(component) > ComponentMaxBytes {
		return Package{}, errors.New("plugin Component exceeds 32 MiB")
	}
	if !bytes.HasPrefix(component, []byte{0, 'a', 's', 'm', 13, 0, 1, 0}) {
		return Package{}, errors.New("plugin requires a WebAssembly Component")
	}
	verified, err := VerifySignature(manifest, component, trust.Keys, trust.AllowUnsigned)
	if err != nil {
		return Package{}, err
	}
	digest := sha256.Sum256(component)
	return Package{Manifest: manifest, Component: component, SHA256: hex.EncodeToString(digest[:]), SignatureVerified: verified}, nil
}

func readPackageFile(root *os.Root, name string, limit int64) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("plugin file is not a regular file within its size limit: " + name)
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, errors.New("could not read plugin " + name)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || opened.Size() > limit || !os.SameFile(info, opened) {
		return nil, errors.New("plugin file is not a regular file within its size limit: " + name)
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("could not read plugin file within its size limit: " + name)
	}
	return data, nil
}
