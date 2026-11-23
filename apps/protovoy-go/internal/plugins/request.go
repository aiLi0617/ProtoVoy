package plugins

import "encoding/json"

type RemoveResult struct {
	ID             string `json:"id"`
	CleanupPending bool   `json:"cleanupPending"`
}

type StateResult struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

const (
	ManifestMaxBytes       = 64 << 10
	ComponentMaxBytes      = 32 << 20
	InstallMessageMaxBytes = 46 << 20
)

type InstallRequest struct {
	Manifest      json.RawMessage `json:"manifest"`
	Component     []byte          `json:"component"`
	AllowUnsigned bool            `json:"allowUnsigned"`
	Permissions   []string        `json:"permissions"`
}

type InstallResult struct {
	ID                string `json:"id"`
	SHA256            string `json:"sha256"`
	SignatureVerified bool   `json:"signatureVerified"`
}

// JSON may escape each input/output byte to six ASCII bytes.
const MessageMaxBytes = (6 << 20) + 8192

type RunRequest struct {
	ExpectedRevision string   `json:"expectedRevision,omitempty"`
	ExpectedVersion  string   `json:"expectedVersion,omitempty"`
	Kind             string   `json:"kind"`
	Input            string   `json:"input"`
	AllowUnsigned    bool     `json:"allowUnsigned"`
	Permissions      []string `json:"permissions"`
}

type RunResult struct {
	Output string `json:"output"`
}
