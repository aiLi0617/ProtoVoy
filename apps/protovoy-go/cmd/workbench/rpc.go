package main

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"strings"

	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"

	"protovoy.dev/protovoy/internal/agentclient"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

var (
	jsonRPCID     = tree.ID(page, "jsonrpc-mode")
	soapID        = tree.ID(page, "soap-mode")
	rpcBackID     = tree.ID(page, "rpc-back")
	rpcMethodID   = tree.ID(page, "rpc-method")
	rpcRequestID  = tree.ID(page, "rpc-request-id")
	soapVersionID = tree.ID(page, "soap-version")
	soapActionID  = tree.ID(page, "soap-action")
)

type rpcPanel struct {
	protocol                    string
	method, id, version, action flex.Field
}

func (panel *rpcPanel) settings() project.RPCSettings {
	return project.RPCSettings{
		Protocol: panel.protocol, Method: strings.TrimSpace(panel.method.Text()),
		ID: strings.TrimSpace(panel.id.Text()), Version: strings.TrimSpace(panel.version.Text()),
		Action: strings.TrimSpace(panel.action.Text()),
	}
}

func (w *workbench) drawRPCModes(f *flex.Frame) {
	if f.Button(jsonRPCID, "JSON-RPC").Clicked() && w.canLeaveDraft() {
		w.loadRequest(0, project.Request{RPC: &project.RPCSettings{Protocol: "jsonrpc"}})
		w.result = result{}
		w.invalidator.Invalidate()
	}
	if f.Button(soapID, "SOAP").Clicked() && w.canLeaveDraft() {
		w.loadRequest(0, project.Request{RPC: &project.RPCSettings{Protocol: "soap"}})
		w.result = result{}
		w.invalidator.Invalidate()
	}
}

func (w *workbench) drawRPC(f *flex.Frame, canvas *ui.Canvas) {
	panel := &w.rpc
	title, bodyLabel, placeholder := "JSON-RPC request", "JSON parameters", "Enter JSON parameters"
	if panel.protocol == "soap" {
		title, bodyLabel, placeholder = "SOAP request", "SOAP envelope", "Enter XML envelope"
	}
	f.Text(title)
	if w.cancel == nil && f.Button(rpcBackID, "Back to requests").Clicked() && w.canLeaveDraft() {
		w.loadRequest(0, project.Request{})
		w.invalidator.Invalidate()
		return
	}
	f.Field(&w.requestName, requestNameID, "Request name", "Request name")
	endpoint := "Server-configured endpoint"
	if localTarget || w.useAgent() {
		endpoint = "http://127.0.0.1:8080/"
	}
	f.Field(&w.target, targetID, "Request URL", endpoint)
	f.Field(&w.httpTimeout, httpTimeoutID, "RPC timeout in milliseconds", "Timeout (ms), default 30000")
	w.drawHTTPAssertions(f)
	f.Row().Gap(8).Children(func() {
		if panel.protocol == "soap" {
			f.Field(&panel.version, soapVersionID, "SOAP version", "1.2 (default) or 1.1").Grow(1)
			f.Field(&panel.action, soapActionID, "SOAP action", "Optional SOAP action").Grow(1)
		} else {
			f.Field(&panel.method, rpcMethodID, "JSON-RPC method", "Method, e.g. lookup").Grow(1)
			f.Field(&panel.id, rpcRequestID, "JSON-RPC ID", "JSON ID (defaults to 1)").Grow(1)
		}
	})
	w.headers.draw(f, canvas, headersID, "Request headers", "Optional HTTP headers").Height(72)
	w.requestBody.draw(f, canvas, requestBodyID, bodyLabel, placeholder).Height(96)
	f.Row().Gap(8).Children(func() {
		if w.cancel != nil {
			if f.Button(cancelID, "Cancel").Clicked() {
				w.cancel()
			}
			return
		}
		label := "Send RPC request"
		if w.useAgent() {
			label = "Send RPC via Local Agent"
		}
		if f.Button(sendID, label).Clicked() {
			w.sendRPC()
		}
		if f.Button(saveRequestID, "Save request").Clicked() {
			w.saveDraft()
		}
		if f.Button(discardID, "Discard edits").Clicked() {
			w.discardDraft()
		}
	})
	w.drawHTTPResponse(f, false)
}

func (w *workbench) sendRPC() {
	draft := w.draft()
	target := strings.TrimSpace(draft.Target)
	headers, err := execution.ParseHeaders(draft.Headers)
	if err == nil && target == "" && (localTarget || w.useAgent() || w.collection.selected != 0) {
		err = errors.New("enter a request URL")
	}
	timeoutMS, timeoutErr := parseRequestTimeout(draft.HTTPTimeoutMS)
	if err == nil {
		err = timeoutErr
	}
	if err != nil {
		w.result = result{status: err.Error(), done: true}
		return
	}
	settings, layers, client := draft.RPC, w.variableLayers(), w.agent.client
	variables := make(map[string]string)
	for _, layer := range layers {
		maps.Copy(variables, layer)
	}
	ctx, cancel := context.WithCancel(w.agentExecutionContext())
	w.cancel = cancel
	w.runningSSE, w.runningSubscription = false, false
	w.result = result{status: "Sending…"}
	go func() {
		defer cancel()
		var payload execution.HTTPPayload
		originalTarget := target
		var err error
		var fields map[string]any
		if settings.Protocol == "jsonrpc" {
			params, id := json.RawMessage(strings.TrimSpace(draft.Body)), json.RawMessage(settings.ID)
			fields = map[string]any{"method": settings.Method}
			if len(params) != 0 {
				fields["params"] = params
			}
			if len(id) != 0 {
				fields["id"] = id
			}
			if client == nil {
				target, payload, err = execution.PrepareJSONRPC(target, settings.Method, params, id, headers, variables)
			}
		} else {
			fields = map[string]any{"version": settings.Version, "action": settings.Action, "envelope": draft.Body}
			if client == nil {
				payload, err = execution.SOAPPayload(settings.Version, settings.Action, draft.Body, headers)
				if err == nil {
					target, payload, err = payload.ResolveVariables(target, variables)
				}
			}
		}
		if err == nil && client == nil && originalTarget != "" && strings.TrimSpace(target) == "" {
			err = errors.New("request URL resolved to an empty value")
		}
		switch {
		case err != nil:
			w.publish(result{status: err.Error(), done: true})
		case client != nil:
			fields["headers"] = headers
			w.publish(readAgentHTTPResult(ctx, false, w.publish, func(update func(agentclient.Response)) (agentclient.Response, error) {
				return client.RunRPC(ctx, target, settings.Protocol, fields, variables, timeoutMS, update)
			}))
		default:
			payload.TimeoutMS = timeoutMS
			w.publish(readResponse(ctx, target, payload, false, w.publish))
		}
	}()
}
