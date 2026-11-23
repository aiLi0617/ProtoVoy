package mock

import (
	"encoding/json"
	"errors"
)

const maxRulesBytes = 8 << 20

// Encode produces a scenario file, including an empty array when no scenarios
// remain after the user deletes the last rule.
func Encode(rules []Rule) ([]byte, error) {
	if rules == nil {
		rules = []Rule{}
	}
	data, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return nil, err
	}
	if len(data) > maxRulesBytes {
		return nil, errors.New("mock rules exceed 8 MiB")
	}
	return data, nil
}
