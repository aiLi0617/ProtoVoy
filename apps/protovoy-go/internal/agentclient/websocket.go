package agentclient

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"protovoy.dev/protovoy/internal/execution"
)

func (client *Client) WebSocketTicket(ctx context.Context, setup execution.WebSocketSetup) (string, []string, error) {
	if !execution.ValidWebSocketTarget(setup.ExpectedTarget) {
		return "", nil, errors.New("enter a valid WebSocket URL for Local Agent execution")
	}
	var response struct{ Ticket string }
	input := struct {
		execution.WebSocketSetup
		RequestID  string `json:"requestId,omitempty"`
		SnapshotID string `json:"snapshotId,omitempty"`
	}{setup, savedRequestID(ctx), historySnapshotID(ctx)}
	if err := client.json(ctx, http.MethodPost, execution.AgentWebSocketTicketPath, input, &response); err != nil {
		return "", nil, err
	}
	data, err := hex.DecodeString(response.Ticket)
	if err != nil || len(data) != 32 {
		return "", nil, errors.New("Local Agent returned an invalid WebSocket ticket")
	}
	endpoint := "ws://" + strings.TrimPrefix(client.origin, "http://") + execution.AgentWebSocketPath
	protocols := []string{execution.AgentWebSocketProtocol, execution.AgentWebSocketTicketPrefix + response.Ticket}
	return endpoint, protocols, nil
}
