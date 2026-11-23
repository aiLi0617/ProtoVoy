package plugins

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
)

func ParseTrustedKeys(value string) (map[string]ed25519.PublicKey, error) {
	keys := map[string]ed25519.PublicKey{}
	if value == "" {
		return keys, nil
	}
	var encoded map[string]string
	if json.Unmarshal([]byte(value), &encoded) != nil {
		return nil, errors.New("invalid PROTOVOY_PLUGIN_TRUSTED_KEYS")
	}
	for id, value := range encoded {
		key, err := base64.StdEncoding.Strict().DecodeString(value)
		if err != nil || len(key) != ed25519.PublicKeySize {
			return nil, errors.New("invalid trusted plugin publisher key")
		}
		keys[id] = key
	}
	return keys, nil
}
