package plugins

const InventoryMaxBytes = 1 << 20

// Installed describes stored metadata, not execution approval or signature trust.
type Installed struct {
	ID       string    `json:"id"`
	Revision string    `json:"revision,omitempty"`
	Disabled bool      `json:"disabled"`
	Manifest *Manifest `json:"manifest,omitempty"`
	Error    string    `json:"error,omitempty"`
}
