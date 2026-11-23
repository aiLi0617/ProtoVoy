package execution

import "encoding/json"

const BrokerResultLimit = 16 << 20

type BrokerBridgeEvent struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message,omitempty"`
	Error   string          `json:"error,omitempty"`
}
