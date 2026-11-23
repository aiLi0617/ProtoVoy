//go:build js && wasm

package main

import (
	"context"
	"maps"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

func (w *workbench) sendScriptedHTTP() {
	if w.useAgent() || w.webSocket || w.sse || w.graphql || w.rpc.protocol != "" || w.datagram != "" {
		w.result = result{status: "Request scripts require HTTP", done: true}
		return
	}
	pre, err := parseRequestScripts(w.preScripts.input.Editor.String())
	var post []string
	if err == nil {
		post, err = parseRequestScripts(w.postScripts.input.Editor.String())
	}
	if err == nil {
		err = (project.Request{PreScripts: pre, PostScripts: post, HTTPSecretReference: w.requestSecret.Text(), HTTPSecretHeader: w.requestSecretHeader.Text()}).ValidateScripts()
	}
	if err != nil {
		w.result = result{status: err.Error(), done: true}
		return
	}
	target, payload, err := w.prepareRequest()
	if err != nil {
		w.result = result{status: err.Error(), done: true}
		return
	}
	variables := map[string]string{}
	for _, layer := range w.variableLayers() {
		maps.Copy(variables, layer)
	}
	input := execution.PreviewRequest{HTTPPayload: payload, ExpectedTarget: target, PreScripts: pre, PostScripts: post, Variables: variables}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.result = result{status: "Sending hosted scripted request…"}
	go func() {
		defer cancel()
		w.publish(readHTTPResponse(ctx, func() (*execution.HTTPResponse, error) { return executePreview(ctx, input) }, false, w.publish))
	}()
}
