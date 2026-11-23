package mock

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

const maxDelayMS = 30000

func Parse(data []byte) ([]Rule, error) {
	if len(data) > maxRulesBytes {
		return nil, errors.New("mock rules exceed 8 MiB")
	}
	var rules []Rule
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&rules) != nil || rules == nil {
		return nil, errors.New("mock rules must be a JSON array with recognized fields")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("mock rules must contain exactly one JSON document")
	}
	hits := make(map[string]uint64, len(rules))
	for i := range rules {
		rule := &rules[i]
		if rule.ID == "" || rule.Method == "" || rule.Status < 200 || rule.Status > 599 || rule.DelayMS > maxDelayMS || rule.WSIntervalMS > maxDelayMS {
			return nil, errors.New("mock rule requires an ID, method, status from 200 to 599, and delay of at most 30000 ms")
		}
		if _, exists := hits[rule.ID]; exists {
			return nil, errors.New("mock rule IDs must be unique")
		}
		if err := rule.Match.validate(); err != nil {
			return nil, err
		}
		for name, value := range rule.Headers {
			if !validHeaderName(name) || strings.ContainsFunc(value, func(c rune) bool {
				return c == 127 || c < 32 && c != '\t'
			}) {
				return nil, errors.New("mock response contains an invalid header")
			}
		}
		if strings.ContainsFunc(rule.ID, func(c rune) bool { return c < 33 || c > 126 }) {
			return nil, errors.New("mock scenario ID must contain printable ASCII without spaces")
		}
		if rule.ProjectKey == "" {
			rule.ProjectKey = "default-project"
		}
		if rule.ServiceKey == "" {
			rule.ServiceKey = "default"
		}
		hits[rule.ID] = 0
	}

	return rules, nil
}
func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
			strings.ContainsRune("!#$%&'*+-.^_`|~", c) {
			continue
		}
		return false
	}
	return true
}
