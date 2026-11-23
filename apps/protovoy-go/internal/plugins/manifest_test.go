package plugins

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestSignatureUsesCanonicalMetadataAndRejectsTampering(t *testing.T) {
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	// The metadata is the lexically sorted JSON produced by serde_json::json!.
	const metadata = "{\"description\":\"<中文>\u2028\\\\u2028\u2029\",\"id\":\"example\",\"kind\":\"transformer\",\"name\":\"Example\",\"permissions\":[],\"publisherKeyId\":\"official\",\"version\":\"1.0.0\"}"
	component := []byte("fixture-component")
	digest := sha256.Sum256(component)
	payload := append([]byte("ProtoVoy Plugin Signature v1\x00"), digest[:]...)
	payload = append(payload, metadata...)
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(key, payload))
	// Generated with the Rust host's serde_json, sha2 and ed25519-dalek
	// signature recipe, including Unicode separators and a literal escape.
	const rustSignature = "gVTTc373JvEWm/Z0uiQDULBU+pKUbbDy1GSRgbLkimUd3ceilCpxbmaVYoKgnjgAE7kQIZftLC2l9VQj3xE+Dw=="
	if signature != rustSignature {
		t.Fatal("canonical metadata diverged from the Rust signature fixture")
	}
	signature = rustSignature
	publisher := "official"
	manifest := Manifest{ID: "example", Name: "Example", Version: "1.0.0", Kind: "transformer",
		Description: "<中文>\u2028\\u2028\u2029", PublisherKeyID: &publisher, SignatureBase64: &signature}
	trusted := map[string]ed25519.PublicKey{publisher: key.Public().(ed25519.PublicKey)}
	if verified, err := VerifySignature(manifest, component, trusted, false); err != nil || !verified {
		t.Fatalf("canonical signature failed: %v", err)
	}
	if _, err := VerifySignature(manifest, []byte("changed"), trusted, false); err == nil {
		t.Fatal("accepted modified component")
	}
	manifest.Name = "Modified"
	if _, err := VerifySignature(manifest, component, trusted, false); err == nil {
		t.Fatal("accepted modified manifest")
	}
}

func TestManifestRejectsTraversalAndInvalidPermissions(t *testing.T) {
	for _, source := range []string{
		`{"id":"../escape","name":"X","version":"1","kind":"transformer"}`,
		`{"id":"example","name":"X","version":"1","kind":"native"}`,
		`{"id":"example","name":"X","version":"1","kind":"transformer","permissions":["execute"]}`,
		`{"id":"example","name":"X","version":"1","kind":"transformer","publisherKeyId":"official"}`,
	} {
		if _, err := ReadManifest([]byte(source)); err == nil {
			t.Fatal("accepted invalid plugin manifest")
		}
	}
}
