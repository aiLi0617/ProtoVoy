package execution

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode"
)

const SQLBridgePath = "/api/sql"

type SQLRequest struct {
	Statement     string            `json:"sql"`
	Parameters    []json.RawMessage `json:"parameters"`
	Transactional bool              `json:"transactional"`
	RowLimit      *int              `json:"rowLimit"`
}

type SQLBridgeRequest struct {
	ExpectedTarget      string `json:"expectedTarget,omitempty"`
	ExpectedPasswordRef string `json:"expectedPasswordRef,omitempty"`
	TimeoutMS           int    `json:"timeoutMs,omitempty"`
	SQLRequest
}

type SQLCredentials struct {
	Username, Password, PasswordRef string
}

func ValidPasswordReference(reference string) bool {
	return len(reference) <= 256 && strings.TrimSpace(reference) == reference &&
		!strings.ContainsFunc(reference, unicode.IsControl)
}

func (request SQLBridgeRequest) ResolveVariables(scopes ...map[string]string) (SQLBridgeRequest, error) {
	resolver := newVariableResolver(scopes)
	request.ExpectedTarget = resolver.text(request.ExpectedTarget)
	request.Statement = resolver.text(request.Statement)
	parameters := make([]json.RawMessage, len(request.Parameters))
	for i, raw := range request.Parameters {
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.UseNumber()
		var value any
		if decoder.Decode(&value) != nil {
			return SQLBridgeRequest{}, errors.New("invalid SQL parameter")
		}
		data, err := json.Marshal(resolver.jsonValue(value, 0))
		if err != nil || len(data) > 1<<20 {
			return SQLBridgeRequest{}, errors.New("SQL parameter exceeds its size limit")
		}
		parameters[i] = data
	}
	request.Parameters = parameters
	return request, resolver.err
}
