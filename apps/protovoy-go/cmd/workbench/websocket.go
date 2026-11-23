package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"protovoy.dev/protovoy/internal/agentclient"
	"protovoy.dev/protovoy/internal/execution"
)

type socketFrame struct {
	kind websocket.MessageType
	data []byte
}

type socketConnector func(context.Context, execution.WebSocketSetup) (*websocket.Conn, string, error)

func (w *workbench) socketConnector() socketConnector {
	if w.useAgent() {
		client := w.agent.client
		return func(ctx context.Context, setup execution.WebSocketSetup) (*websocket.Conn, string, error) {
			return connectAgentWebSocket(ctx, client, setup)
		}
	}
	return connectWebSocket
}

func connectAgentWebSocket(ctx context.Context, client *agentclient.Client, setup execution.WebSocketSetup) (*websocket.Conn, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	endpoint, protocols, err := client.WebSocketTicket(ctx, setup)
	if err != nil {
		return nil, "", err
	}
	connection, err := dialAgentSocket(ctx, endpoint, protocols)
	if err != nil {
		return nil, "", fmt.Errorf("could not open the Local Agent WebSocket")
	}
	if connection.Subprotocol() != execution.AgentWebSocketProtocol {
		closeWorkbenchSocket(connection)
		return nil, "", fmt.Errorf("Local Agent did not negotiate the bridge protocol")
	}
	return awaitSocketReady(ctx, connection)
}

func awaitSocketReady(ctx context.Context, connection *websocket.Conn) (*websocket.Conn, string, error) {
	connection.SetReadLimit(execution.WebSocketMessageLimit)
	var ready execution.WebSocketReady
	if err := wsjson.Read(ctx, connection, &ready); err != nil {
		closeWorkbenchSocket(connection)
		return nil, "", fmt.Errorf("WebSocket bridge handshake failed")
	}
	if ready.Error != "" {
		closeWorkbenchSocket(connection)
		return nil, "", fmt.Errorf("%s", ready.Error)
	}
	return connection, ready.Subprotocol, nil
}

func (w *workbench) startSocket() {
	draft := w.draft()
	target := strings.TrimSpace(draft.Target)
	if target == "" && (localTarget || w.useAgent() || w.collection.selected != 0) {
		w.result = result{status: "enter a WebSocket URL", done: true}
		return
	}
	headers, err := execution.ParseHeaders(draft.Headers)
	if err != nil {
		w.result = result{status: err.Error(), done: true}
		return
	}
	setup, err := execution.PrepareWebSocket(target, headers, draft.WebSocketProtocols, w.variableLayers()...)
	if err != nil {
		w.result = result{status: err.Error(), done: true}
		return
	}
	options, err := parseWebSocketAutomation(draft.WebSocketAutomation)
	if err != nil {
		w.result = result{status: err.Error(), done: true}
		return
	}
	data, err := options.Frames(w.variableLayers()...)
	if err != nil {
		w.result = result{status: err.Error(), done: true}
		return
	}
	frames := make([]socketFrame, len(data))
	for index, payload := range data {
		kind := websocket.MessageText
		if options.Messages[index].Encoding == "binary" {
			kind = websocket.MessageBinary
		}
		frames[index] = socketFrame{kind, payload}
	}
	ctx, cancel := context.WithCancel(w.agentExecutionContext())
	outbox := make(chan socketFrame, 8)
	w.cancel, w.socketOutbox = cancel, outbox
	w.runningSSE, w.runningSubscription = false, false
	w.socketNotice = ""
	w.socketOptionsOpen = false
	w.result = result{status: "Connecting WebSocket…"}
	connect := w.socketConnector()
	go func() {
		defer cancel()
		w.publish(readSocket(ctx, connect, setup, outbox, options, frames, w.publish))
	}()
}

func parseWebSocketAutomation(input string) (execution.WebSocketAutomation, error) {
	var options execution.WebSocketAutomation
	if strings.TrimSpace(input) == "" {
		return options, nil
	}
	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.DisallowUnknownFields()
	if !strings.HasPrefix(strings.TrimSpace(input), "{") || decoder.Decode(&options) != nil || decoder.Decode(new(any)) != io.EOF {
		return options, errors.New("enter WebSocket connection options as a JSON object")
	}
	return options, nil
}

func (w *workbench) sendFrame(kind websocket.MessageType) {
	if !w.result.socketReady || w.socketOutbox == nil {
		return
	}
	payload := execution.HTTPPayload{Body: w.requestBody.input.Editor.String()}
	_, payload, err := payload.ResolveVariables("", w.variableLayers()...)
	if err != nil {
		w.socketNotice = err.Error()
		return
	}
	var data []byte
	if kind == websocket.MessageBinary {
		if len(payload.Body) > base64.StdEncoding.EncodedLen(execution.WebSocketMessageLimit)+4096 {
			w.socketNotice = "WebSocket message exceeds 256 KiB"
			return
		}
		data, err = base64.StdEncoding.DecodeString(strings.TrimSpace(payload.Body))
		if err != nil {
			w.socketNotice = "enter Base64 for a binary WebSocket message"
			return
		}
	} else {
		data = []byte(payload.Body)
	}
	if len(data) > execution.WebSocketMessageLimit {
		w.socketNotice = "WebSocket message exceeds 256 KiB"
		return
	}
	select {
	case w.socketOutbox <- socketFrame{kind, data}:
		w.socketNotice = ""
	default:
		w.socketNotice = "WebSocket send queue is full"
	}
}

type socketRun struct {
	options  execution.WebSocketAutomation
	startup  []socketFrame
	entries  []string
	received int
}

func readSocket(ctx context.Context, connect socketConnector, setup execution.WebSocketSetup, outbox <-chan socketFrame,
	options execution.WebSocketAutomation, frames []socketFrame, publish func(result)) result {
	run := socketRun{options: options, startup: frames}
	for attempt := 0; ; attempt++ {
		value, reconnect := run.session(ctx, connect, setup, outbox, publish)
		if !reconnect || attempt >= options.ReconnectMax {
			return value
		}
		publish(result{status: fmt.Sprintf("WebSocket reconnecting · %d/%d", attempt+1, options.ReconnectMax), body: value.body})
		timer := time.NewTimer(options.ReconnectDelay())
		select {
		case <-ctx.Done():
			timer.Stop()
			return result{status: "WebSocket disconnected", body: value.body, done: true}
		case <-timer.C:
		}
	}
}

func (run *socketRun) session(ctx context.Context, connect socketConnector, setup execution.WebSocketSetup, outbox <-chan socketFrame, publish func(result)) (result, bool) {
	body := strings.Join(run.entries, "\n\n")
	connection, subprotocol, err := connect(ctx, setup)
	if ctx.Err() != nil {
		if connection != nil {
			closeWorkbenchSocket(connection)
		}
		return result{status: "WebSocket disconnected", body: body, done: true}, false
	}
	if err != nil {
		return result{status: err.Error(), body: body, done: true}, true
	}
	defer closeWorkbenchSocket(connection)
	readCtx, stopRead := context.WithCancel(ctx)
	defer stopRead()
	inbox := make(chan socketFrame, 8)
	readDone := make(chan error, 1)
	go func() {
		for {
			nextCtx, finish := readCtx, func() {}
			if run.options.IdleTimeoutMS > 0 {
				nextCtx, finish = context.WithTimeout(readCtx, time.Duration(run.options.IdleTimeoutMS)*time.Millisecond)
			}
			kind, data, err := connection.Read(nextCtx)
			finish()
			if err != nil {
				readDone <- err
				return
			}
			select {
			case inbox <- socketFrame{kind, data}:
			case <-readCtx.Done():
				return
			}
		}
	}()
	value := result{status: "WebSocket connected", body: body}
	if subprotocol != "" {
		value.status += " · " + subprotocol
	}
	publish(value)
	add := func(direction string, frame socketFrame) {
		data := frame.data
		const maxPreview = 2048
		truncated := len(data) > maxPreview
		data = data[:min(len(data), maxPreview)]
		kind := "text"
		text := strings.ToValidUTF8(string(data), "�")
		if frame.kind == websocket.MessageBinary {
			kind, text = "binary (Base64)", base64.StdEncoding.EncodeToString(data)
		}
		if truncated {
			text += "…"
		}
		const maxEntries = 20
		if len(run.entries) == maxEntries {
			copy(run.entries, run.entries[1:])
			run.entries = run.entries[:maxEntries-1]
		}
		run.entries = append(run.entries, direction+" "+kind+"\n"+text)
		value.body = strings.Join(run.entries, "\n\n")
		publish(value)
	}
	write := func(frame socketFrame) error {
		writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := connection.Write(writeCtx, frame.kind, frame.data); err != nil {
			return err
		}
		add("Sent", frame)
		return nil
	}
	writeFailure := func() result {
		status := "could not send WebSocket message"
		if ctx.Err() != nil {
			status = "WebSocket disconnected"
		}
		return result{status: status, body: value.body, done: true}
	}
	for _, frame := range run.startup {
		if write(frame) != nil {
			return writeFailure(), false
		}
	}
	limitReached := func() bool {
		return run.options.ReceiveLimit != nil && run.received >= *run.options.ReceiveLimit
	}
	value.socketReady = true
	publish(value)
	readEnded := func(err error) (result, bool) {
		status := "WebSocket connection ended"
		if ctx.Err() != nil {
			status = "WebSocket disconnected"
		} else if code := websocket.CloseStatus(err); code != -1 {
			status = fmt.Sprintf("WebSocket closed · %d", code)
		}
		return result{status: status, body: value.body, done: true}, ctx.Err() == nil
	}
	for !limitReached() {
		select {
		case <-ctx.Done():
			return readEnded(ctx.Err())
		case err := <-readDone:
			for !limitReached() && len(inbox) > 0 {
				run.received++
				add("Received", <-inbox)
			}
			if !limitReached() {
				return readEnded(err)
			}
		case frame := <-inbox:
			run.received++
			add("Received", frame)
		case frame := <-outbox:
			if write(frame) != nil {
				return writeFailure(), false
			}
		}
	}
	return result{status: fmt.Sprintf("WebSocket completed · %d received", run.received), body: value.body, done: true}, false
}
