package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/coder/websocket"

	"protovoy.dev/protovoy/internal/execution"
)

func (w *workbench) startSubscription() {
	draft := w.draft()
	target := strings.TrimSpace(draft.Target)
	if target == "" && (localTarget || w.useAgent() || w.collection.selected != 0) {
		w.result = result{status: "enter a GraphQL subscription URL", done: true}
		return
	}
	headers, err := execution.ParseHeaders(draft.Headers)
	if err != nil {
		w.result = result{status: err.Error(), done: true}
		return
	}
	document := execution.GraphQLDocument{Query: draft.Body, Variables: draft.GraphQLVariables, OperationName: draft.GraphQLOperation}
	setup, payload, err := document.PrepareSubscription(target, headers, w.variableLayers()...)
	if err != nil {
		w.result = result{status: err.Error(), done: true}
		return
	}
	ctx, cancel := context.WithCancel(w.agentExecutionContext())
	w.cancel = cancel
	w.runningSSE, w.runningSubscription = false, true
	w.result = result{status: "Connecting GraphQL subscription…"}
	connect := w.socketConnector()
	go func() {
		defer cancel()
		w.publish(readSubscription(ctx, connect, setup, payload, w.publish))
	}()
}

func readSubscription(ctx context.Context, connect socketConnector, setup execution.WebSocketSetup, payload json.RawMessage, publish func(result)) result {
	connection, protocol, err := connect(ctx, setup)
	if connection != nil {
		defer closeWorkbenchSocket(connection)
	}
	if ctx.Err() != nil {
		return result{status: "GraphQL subscription stopped", done: true}
	}
	if err != nil {
		return result{status: err.Error(), done: true}
	}
	if protocol != execution.GraphQLWebSocketProtocol {
		return result{status: "Server did not negotiate graphql-transport-ws", done: true}
	}
	var value result
	var entries []string
	total := 0
	err = execution.RunGraphQLSubscription(ctx, connection, payload, func() {
		value.status = "GraphQL subscription active"
		publish(value)
	}, func(data json.RawMessage, rejected bool) {
		total++
		const maxPreview = 2048
		var formatted bytes.Buffer
		if len(data) <= maxPreview && json.Indent(&formatted, data, "", "  ") == nil {
			data = formatted.Bytes()
		}
		preview := strings.ToValidUTF8(string(data[:min(len(data), maxPreview)]), "�")
		if len(data) > maxPreview {
			preview += "…"
		}
		label := fmt.Sprintf("Result %d", total)
		if rejected {
			label = "GraphQL subscription error"
		} else if count := execution.GraphQLErrorCount(data); count != 0 {
			label += fmt.Sprintf(" · GraphQL errors: %d", count)
		}
		const visibleResults = 20
		if len(entries) == visibleResults {
			copy(entries, entries[1:])
			entries = entries[:visibleResults-1]
		}
		entries = append(entries, label+"\n"+preview)
		value.body = strings.Join(entries, "\n\n")
		publish(value)
	})
	switch {
	case ctx.Err() != nil:
		value.status = "GraphQL subscription stopped"
	case errors.Is(err, execution.ErrGraphQLSubscriptionRejected):
		value.status = "GraphQL subscription rejected"
	case errors.Is(err, context.DeadlineExceeded):
		value.status = "GraphQL subscription acknowledgement timed out"
	case err == nil:
		value.status = fmt.Sprintf("GraphQL subscription completed · %d results", total)
	default:
		if code := websocket.CloseStatus(err); code != -1 {
			value.status = fmt.Sprintf("GraphQL subscription connection closed · %d", code)
		} else {
			value.status = "GraphQL subscription connection failed"
		}
	}
	value.done = true
	return value
}
