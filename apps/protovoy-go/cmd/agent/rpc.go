package main

import (
	"encoding/json"
	"errors"
	"time"

	"protovoy.dev/protovoy/internal/execution"
)

func (envelope requestEnvelope) prepareRPC(timeout time.Duration) (execution.HTTPRequest, error) {
	var raw struct {
		Type  string          `json:"type"`
		Value json.RawMessage `json:"value"`
	}
	// Older raw envelopes also serialize their fields next to "type".
	if json.Unmarshal(envelope.Payload, &raw) != nil || raw.Type != "raw" {
		return execution.HTTPRequest{}, errors.New("invalid RPC execution payload")
	}
	payload := raw.Value
	if len(payload) == 0 {
		payload = envelope.Payload
	} else if !decodeExecutionPayload(envelope.Payload, &raw) {
		return execution.HTTPRequest{}, errors.New("invalid RPC execution payload")
	}
	target := envelope.Target
	var httpPayload execution.HTTPPayload
	var err error
	if envelope.Protocol == "jsonrpc" {
		var input struct {
			Type    string          `json:"type"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
			ID      json.RawMessage `json:"id"`
			Headers [][2]string     `json:"headers"`
		}
		if !decodeExecutionPayload(payload, &input) {
			return execution.HTTPRequest{}, errors.New("invalid JSON-RPC payload")
		}
		target, httpPayload, err = execution.PrepareJSONRPC(target, input.Method, input.Params, input.ID, input.Headers, envelope.Variables)
	} else {
		var input struct {
			Type     string      `json:"type"`
			Version  string      `json:"version"`
			Action   string      `json:"action"`
			Envelope string      `json:"envelope"`
			Headers  [][2]string `json:"headers"`
		}
		if !decodeExecutionPayload(payload, &input) {
			return execution.HTTPRequest{}, errors.New("invalid SOAP payload")
		}
		httpPayload, err = execution.SOAPPayload(input.Version, input.Action, input.Envelope, input.Headers)
		if err == nil {
			target, httpPayload, err = httpPayload.ResolveVariables(target, envelope.Variables)
		}
	}
	if err != nil {
		return execution.HTTPRequest{}, err
	}
	return prepareResolvedHTTP(target, httpPayload, false, timeout)
}
