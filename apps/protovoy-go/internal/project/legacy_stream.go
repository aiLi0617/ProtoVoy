package project

import (
	"encoding/json"
	"errors"
	"strings"

	"protovoy.dev/protovoy/internal/execution"
)

func readLegacySocket(protocol string, data []byte, metadata map[string]json.RawMessage, request *Request) error {
	var input struct {
		execution.SocketPayload
		Type, CACertRef string
	}
	if !decodeLegacy(data, &input) || input.Type != protocol || input.Network != "" || input.ExpectedTarget != "" ||
		!input.ValidSendOptions() {
		return errors.New("invalid legacy socket payload")
	}
	if input.CACertRef != "" || hasPrivateKey(input.CAPEM) {
		return errors.New("legacy socket certificate reference requires manual migration")
	}
	for key, expected := range map[string]string{"socketPayloadFormat": input.Encoding, "socketSourceData": input.Data} {
		if raw, ok := metadata[key]; ok {
			var value string
			if json.Unmarshal(raw, &value) != nil || value != expected {
				return errors.New("legacy socket source format cannot be preserved")
			}
		}
	}
	switch input.Encoding {
	case "", "text", "base64", "hex":
	default:
		return errors.New("unsupported legacy socket encoding")
	}
	if input.Framing == "none" {
		input.Framing = "raw"
	}
	switch input.Framing {
	case "", "raw":
		if input.Delimiter != "" || input.FixedLength != 0 {
			return errors.New("unexpected legacy socket framing options")
		}
	case "fixed":
		if protocol != "tcp" || input.FixedLength < 1 || input.FixedLength > execution.SocketResponseLimit || input.Delimiter != "" {
			return errors.New("invalid legacy fixed-length framing")
		}
	case "delimiter":
		if protocol != "tcp" || len(input.Delimiter) == 0 || len(input.Delimiter) > 4096 || input.FixedLength != 0 {
			return errors.New("invalid legacy delimiter framing")
		}
	default:
		return errors.New("unsupported legacy socket framing")
	}
	if protocol == "udp" && input.TLS || !input.TLS && (input.ServerName != "" || input.CAPEM != "") {
		return errors.New("invalid legacy socket TLS settings")
	}
	input.Network = protocol
	request.URL = strings.TrimPrefix(request.URL, protocol+"://")
	if strings.Contains(request.URL, "@") {
		return errors.New("remove credentials from the legacy socket target")
	}
	request.Socket = &input.SocketPayload
	return nil
}

func readLegacyStream(protocol string, data []byte, request *Request) error {
	if protocol == "sse" {
		var input struct {
			Type             string
			Headers          [][2]string
			LastEventID      *string
			ReconnectMax     int
			ReconnectDelayMS *int
		}
		if !decodeLegacy(data, &input) || input.Type != protocol {
			return errors.New("invalid legacy SSE payload")
		}
		options := execution.SSEOptions{Reconnects: input.ReconnectMax, ReconnectDelayMS: input.ReconnectDelayMS}
		if !options.Valid() {
			return errors.New("invalid legacy SSE reconnect settings")
		}
		if input.LastEventID != nil {
			if strings.ContainsAny(*input.LastEventID, "\r\n\x00") {
				return errors.New("invalid legacy SSE event ID")
			}
			input.Headers = execution.SSEHeadersWithEventID(input.Headers, *input.LastEventID)
		}
		headers, err := uniqueHeaderMap(input.Headers)
		if err != nil {
			return err
		}
		request.Headers, request.SSE = headers, &options
		return nil
	}
	var input struct {
		Type             string
		Headers          [][2]string
		Subprotocols     []string
		Messages         []execution.WebSocketMessage
		ReceiveLimit     *int
		ReconnectMax     int
		ReconnectDelayMS *int
	}
	if !decodeLegacy(data, &input) || input.Type != protocol {
		return errors.New("invalid legacy WebSocket payload")
	}
	if err := (execution.WebSocketSetup{Subprotocols: input.Subprotocols}).Validate(); err != nil {
		return err
	}
	subprotocols := strings.Join(input.Subprotocols, ", ")
	headers, err := uniqueHeaderMap(input.Headers)
	if err != nil {
		return err
	}
	request.Headers, request.WebSocket = headers, &execution.WebSocketOptions{
		Subprotocols: subprotocols, WebSocketAutomation: execution.WebSocketAutomation{
			Messages: input.Messages, ReceiveLimit: input.ReceiveLimit, ReconnectMax: input.ReconnectMax,
			ReconnectDelayMS: input.ReconnectDelayMS, IdleTimeoutMS: 30_000,
		},
	}
	return nil
}
