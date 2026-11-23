package agentclient

import (
	"context"
	"encoding/base64"
	"errors"
	"time"

	"protovoy.dev/protovoy/internal/execution"
)

func (client *Client) RunSocket(ctx context.Context, payload execution.SocketPayload) ([]byte, error) {
	if !execution.ValidRequestTimeout(payload.TimeoutMS) {
		return nil, errors.New("invalid socket timeout")
	}
	timeout := execution.RequestTimeout(payload.TimeoutMS)
	ctx, cancel := context.WithTimeout(ctx, timeout+5*time.Second)
	defer cancel()
	input := map[string]any{
		"type": payload.Network, "data": payload.Data, "encoding": payload.Encoding,
		"framing": payload.Framing, "delimiter": payload.Delimiter, "fixedLength": payload.FixedLength,
		"tls": payload.TLS, "serverName": payload.ServerName, "caPem": payload.CAPEM,
		"intervalMs": payload.IntervalMS,
	}
	if payload.SendCount != 0 {
		input["sendCount"] = payload.SendCount
	}
	stream, err := client.openExecutionWithVariables(ctx, payload.ExpectedTarget, payload.Network, input, nil, int(timeout/time.Millisecond))
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	return stream.readBinary(execution.SocketResponseLimit, "socket")
}

func (stream *executionStream) readBinary(limit int, protocol string) ([]byte, error) {
	decoder := execution.NewSSEDecoder(stream.body, "", 0)
	var response []byte
	for {
		event, err := nextAgentEvent(decoder)
		if err != nil {
			return response, err
		}
		switch event.Type {
		case "response_chunk":
			if event.DataBase64 == "" {
				continue
			}
			chunk, err := base64.StdEncoding.DecodeString(event.DataBase64)
			if err != nil || len(chunk) > limit-len(response) {
				return nil, errors.New("Local Agent returned an invalid or oversized " + protocol + " response")
			}
			response = append(response, chunk...)
		case "completed":
			stream.finished = true
			return response, nil
		case "failed":
			stream.finished = true
			if event.Code == "timeout" {
				return response, context.DeadlineExceeded
			}
			return response, errors.New("Local Agent " + protocol + " execution failed")
		case "cancelled":
			stream.finished = true
			return response, context.Canceled
		}
	}
}
