package execution

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

func PrepareJSONRPC(target, method string, params, id json.RawMessage, headers [][2]string, scopes ...map[string]string) (string, HTTPPayload, error) {
	if len(params) == 0 {
		params = json.RawMessage("{}")
	}
	if len(id) == 0 {
		id = json.RawMessage("1")
	}
	resolver := newVariableResolver(scopes)
	resolve := func(raw json.RawMessage) (any, error) {
		if !json.Valid(raw) {
			return nil, errors.New("invalid JSON-RPC parameters or ID")
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, errors.New("invalid JSON-RPC parameters or ID")
		}
		return resolver.jsonValue(value, 0), resolver.err
	}
	target, method, headers = resolver.text(target), resolver.text(method), resolver.headers(headers)
	if strings.TrimSpace(method) == "" && resolver.err == nil {
		return "", HTTPPayload{}, errors.New("JSON-RPC method is required")
	}
	parameters, err := resolve(params)
	if err != nil {
		return "", HTTPPayload{}, err
	}
	switch parameters.(type) {
	case nil, []any, map[string]any:
	default:
		return "", HTTPPayload{}, errors.New("JSON-RPC parameters must be an object or array")
	}
	requestID, err := resolve(id)
	if err != nil {
		return "", HTTPPayload{}, err
	}
	switch requestID.(type) {
	case nil, string, json.Number:
	default:
		return "", HTTPPayload{}, errors.New("JSON-RPC ID must be a string, number, or null")
	}
	body, err := json.Marshal(struct {
		Version string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  any    `json:"params"`
		ID      any    `json:"id"`
	}{"2.0", method, parameters, requestID})
	if err != nil || len(body) > 1<<20 {
		return "", HTTPPayload{}, errors.New("could not encode JSON-RPC request within 1 MiB")
	}
	return target, HTTPPayload{Method: http.MethodPost, Headers: defaultHeader(headers, "Content-Type", "application/json"), Body: string(body)}, nil
}

func SOAPPayload(version, action, envelope string, headers [][2]string) (HTTPPayload, error) {
	if strings.TrimSpace(envelope) == "" {
		return HTTPPayload{}, errors.New("SOAP envelope is required")
	}
	var contentType string
	switch version {
	case "1.1":
		contentType = "text/xml; charset=utf-8"
	case "", "1.2":
		contentType = "application/soap+xml; charset=utf-8"
	default:
		return HTTPPayload{}, errors.New("select SOAP 1.1 or 1.2")
	}
	headers = defaultHeader(headers, "Content-Type", contentType)
	if action != "" {
		headers = defaultHeader(headers, "SOAPAction", action)
	}
	return HTTPPayload{Method: http.MethodPost, Headers: headers, Body: envelope}, nil
}
