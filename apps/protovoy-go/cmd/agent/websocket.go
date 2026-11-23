package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"protovoy.dev/protovoy/internal/agenthistory"
	"protovoy.dev/protovoy/internal/execution"
)

type socketTicket struct {
	setup      execution.WebSocketSetup
	requestID  string
	snapshotID string
	origin     string
	expires    time.Time
	expiry     *time.Timer
}

func (agent *agentService) createWebSocketTicket(w http.ResponseWriter, r *http.Request) {
	const lifetime = 15 * time.Second
	var input struct {
		execution.WebSocketSetup
		RequestID  string `json:"requestId"`
		SnapshotID string `json:"snapshotId"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, execution.WebSocketSetupLimit))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF ||
		input.RequestID != "" && !validID(input.RequestID) ||
		input.SnapshotID != "" && !validID(input.SnapshotID) ||
		!execution.ValidWebSocketTarget(input.ExpectedTarget) || input.Validate() != nil {
		http.Error(w, "invalid WebSocket setup", http.StatusBadRequest)
		return
	}
	var random [32]byte
	rand.Read(random[:])
	token := hex.EncodeToString(random[:])
	now := time.Now()
	agent.mu.Lock()
	for key, entry := range agent.socketTickets {
		if !now.Before(entry.expires) {
			delete(agent.socketTickets, key)
			entry.expiry.Stop()
		}
	}
	if len(agent.socketTickets) >= 64 {
		agent.mu.Unlock()
		http.Error(w, "too many pending WebSocket tickets", http.StatusTooManyRequests)
		return
	}
	key := sha256.Sum256([]byte(token))
	expiry := time.AfterFunc(lifetime, func() {
		agent.mu.Lock()
		delete(agent.socketTickets, key)
		agent.mu.Unlock()
	})
	agent.socketTickets[key] = socketTicket{setup: input.WebSocketSetup, requestID: input.RequestID, snapshotID: input.SnapshotID,
		origin: r.Header.Get("Origin"), expires: now.Add(lifetime), expiry: expiry}
	agent.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ticket": token, "expiresInSeconds": int(lifetime.Seconds())})
}

func (agent *agentService) upgradeWebSocket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "use GET for WebSocket upgrades", http.StatusMethodNotAllowed)
		return
	}
	token, ok := webSocketTicketToken(r.Header)
	if !ok {
		http.Error(w, "WebSocket ticket required", http.StatusUnauthorized)
		return
	}
	select {
	case agent.socketSlots <- struct{}{}:
		defer func() { <-agent.socketSlots }()
	default:
		http.Error(w, "too many active WebSocket connections", http.StatusServiceUnavailable)
		return
	}
	key := sha256.Sum256([]byte(token))
	agent.mu.Lock()
	ticket, found := agent.socketTickets[key]
	authorized := found && time.Now().Before(ticket.expires) && ticket.origin == r.Header.Get("Origin")
	if authorized {
		delete(agent.socketTickets, key)
		ticket.expiry.Stop()
	}
	agent.mu.Unlock()
	if !authorized {
		http.Error(w, "invalid or expired WebSocket ticket", http.StatusUnauthorized)
		return
	}
	// The ticket already matched the exact origin. Escape IPv6 brackets for
	// the library's path-pattern matcher as well.
	origin := strings.NewReplacer("[", "\\[", "]", "\\]").Replace(ticket.origin)
	client, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols: []string{execution.AgentWebSocketProtocol}, OriginPatterns: []string{origin},
	})
	if err != nil {
		return
	}
	started := time.Now()
	received, relayErr := execution.RelayWebSocket(context.Background(), client, ticket.setup.ExpectedTarget, ticket.setup)
	client.CloseNow()
	finished := time.Now()
	state := "failed"
	switch websocket.CloseStatus(relayErr) {
	case websocket.StatusNormalClosure, websocket.StatusGoingAway:
		state = "completed"
	}
	record := agenthistory.Record{
		ID: newExecutionID(), RequestID: ticket.requestID, SnapshotID: ticket.snapshotID, ProtocolID: "websocket", State: state, BytesReceived: received,
		StartedAt: started.UTC(), FinishedAt: finished.UTC(), DurationMS: finished.Sub(started).Milliseconds(),
	}
	if err := agent.history.save(record, nil); err != nil {
		log.Print("could not save Local Agent WebSocket history")
	}
}

func webSocketTicketToken(headers http.Header) (string, bool) {
	var token string
	var protocol bool
	for _, list := range headers.Values("Sec-WebSocket-Protocol") {
		for item := range strings.SplitSeq(list, ",") {
			item = strings.TrimSpace(item)
			if item == execution.AgentWebSocketProtocol {
				protocol = true
			}
			if value, found := strings.CutPrefix(item, execution.AgentWebSocketTicketPrefix); found {
				if token != "" || len(value) != 64 {
					return "", false
				}
				token = value
			}
		}
	}
	return token, protocol && token != ""
}
