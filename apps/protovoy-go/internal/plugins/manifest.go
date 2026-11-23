package plugins

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

type Manifest struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Version         string   `json:"version"`
	Kind            string   `json:"kind"`
	Permissions     []string `json:"permissions"`
	Description     string   `json:"description"`
	PublisherKeyID  *string  `json:"publisherKeyId,omitempty"`
	SignatureBase64 *string  `json:"signatureBase64,omitempty"`
}

func ReadManifest(data []byte) (Manifest, error) {
	var manifest Manifest
	if len(data) > 64<<10 || !utf8.Valid(data) || json.Unmarshal(data, &manifest) != nil {
		return Manifest{}, errors.New("invalid plugin manifest")
	}
	if !ValidID(manifest.ID) || strings.TrimSpace(manifest.Name) == "" || strings.TrimSpace(manifest.Version) == "" {
		return Manifest{}, errors.New("invalid plugin identity")
	}
	switch manifest.Kind {
	case "protocol", "auth", "importer", "transformer":
	default:
		return Manifest{}, errors.New("invalid plugin kind")
	}
	for _, permission := range manifest.Permissions {
		switch permission {
		case "network", "filesystem_read", "filesystem_write", "secrets_read":
		default:
			return Manifest{}, errors.New("invalid plugin permission")
		}
	}
	if (manifest.PublisherKeyID == nil) != (manifest.SignatureBase64 == nil) {
		return Manifest{}, errors.New("plugin publisher and signature must be supplied together")
	}
	return manifest, nil
}

func ValidID(id string) bool {
	return id != "" && len(id) <= 80 && !strings.ContainsFunc(id, func(c rune) bool {
		return !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_')
	})
}

func VerifySignature(manifest Manifest, component []byte, trusted map[string]ed25519.PublicKey, allowUnsigned bool) (bool, error) {
	if manifest.PublisherKeyID == nil && manifest.SignatureBase64 == nil && allowUnsigned {
		return false, nil
	}
	if manifest.PublisherKeyID == nil || manifest.SignatureBase64 == nil {
		return false, errors.New("plugin requires a trusted publisher signature")
	}
	key := trusted[*manifest.PublisherKeyID]
	signature, err := base64.StdEncoding.Strict().DecodeString(*manifest.SignatureBase64)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return false, errors.New("invalid plugin publisher or signature")
	}
	permissions := manifest.Permissions
	if permissions == nil {
		permissions = []string{}
	}
	// Rust's serde_json map serializes signature metadata in lexical key order.
	metadata := map[string]any{"id": manifest.ID, "name": manifest.Name, "version": manifest.Version,
		"kind": manifest.Kind, "permissions": permissions, "description": manifest.Description, "publisherKeyId": manifest.PublisherKeyID}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(metadata); err != nil {
		return false, err
	}
	digest := sha256.Sum256(component)
	payload := append([]byte("ProtoVoy Plugin Signature v1\x00"), digest[:]...)
	payload = append(payload, serdeJSONSeparators(bytes.TrimSuffix(encoded.Bytes(), []byte("\n")))...)
	if !ed25519.Verify(key, payload, signature) {
		return false, errors.New("plugin signature verification failed")
	}
	return true, nil
}

// serde_json emits Unicode line separators literally. Go escapes them even
// with HTML escaping disabled. Preserve escaped backslashes in literal text.
func serdeJSONSeparators(data []byte) []byte {
	var output []byte
	for i := 0; i < len(data); {
		if data[i] == '\\' && i+1 < len(data) {
			if i+6 <= len(data) {
				switch string(data[i : i+6]) {
				case "\\u2028":
					output = append(output, []byte("\u2028")...)
					i += 6
					continue
				case "\\u2029":
					output = append(output, []byte("\u2029")...)
					i += 6
					continue
				}
			}
			output = append(output, data[i], data[i+1])
			i += 2
			continue
		}
		output = append(output, data[i])
		i++
	}
	return output
}
