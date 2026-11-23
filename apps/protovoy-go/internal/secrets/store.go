//go:build !js

package secrets

import (
	"errors"

	"github.com/zalando/go-keyring"
	"protovoy.dev/protovoy/internal/execution"
)

const service = "protovoy.go.secrets"

var ErrNotFound = errors.New("secret reference was not found")

func validReference(reference string) bool {
	return reference != "" && execution.ValidPasswordReference(reference)
}

func Put(reference, value string) error {
	if !validReference(reference) || value == "" || len(value) > 2560 {
		return errors.New("secret requires a valid reference and 1 to 2560 bytes")
	}
	if err := keyring.Set(service, reference, value); err != nil {
		return errors.New("could not write the system credential store")
	}
	return nil
}

func Resolve(reference string) (string, error) {
	if !validReference(reference) {
		return "", errors.New("invalid secret reference")
	}
	value, err := keyring.Get(service, reference)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", errors.New("could not read the system credential store")
	}
	return value, nil
}

func Delete(reference string) error {
	if !validReference(reference) {
		return errors.New("invalid secret reference")
	}
	err := keyring.Delete(service, reference)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return errors.New("could not delete from the system credential store")
	}
	return nil
}
