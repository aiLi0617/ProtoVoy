//go:build !js

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"time"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
	"protovoy.dev/protovoy/internal/scripting"
)

func (w *workbench) sendScriptedHTTP() {
	if w.useAgent() || w.webSocket || w.sse || w.graphql || w.rpc.protocol != "" || w.datagram != "" {
		w.result = result{status: "Select direct desktop HTTP execution for request scripts", done: true}
		return
	}
	pre, err := parseRequestScripts(w.preScripts.input.Editor.String())
	if err != nil {
		w.result = result{status: "Pre-request scripts: " + err.Error(), done: true}
		return
	}
	post, err := parseRequestScripts(w.postScripts.input.Editor.String())
	if err == nil {
		scripts := project.Request{PreScripts: pre, PostScripts: post,
			HTTPSecretReference: w.requestSecret.Text(), HTTPSecretHeader: w.requestSecretHeader.Text()}
		err = scripts.ValidateScripts()
	}
	if err != nil {
		w.result = result{status: err.Error(), done: true}
		return
	}
	target, payload, err := w.prepareRequest()
	if err == nil {
		payload, err = payload.EncodeBody()
	}
	if err != nil {
		w.result = result{status: err.Error(), done: true}
		return
	}
	variables := make(map[string]string)
	for _, layer := range w.variableLayers() {
		maps.Copy(variables, layer)
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.result = result{status: "Sending scripted request…"}
	go func() {
		defer cancel()
		open := func() (*execution.HTTPResponse, error) {
			input, data, scope, err := scripting.PrepareHTTP(ctx, payload.Request(target), pre, variables)
			if err != nil {
				return nil, err
			}
			start := time.Now()
			response, err := (execution.HTTPClient{}).Execute(ctx, input)
			if err != nil || len(post) == 0 {
				return response, err
			}
			defer response.Body.Close()
			body, err := scripting.ReadHTTPPost(ctx, response, post, data, scope, start)
			if err != nil {
				return nil, fmt.Errorf("HTTP %d · %w", response.StatusCode, err)
			}
			return &execution.HTTPResponse{
				StatusCode: response.StatusCode, Headers: response.Headers, Body: io.NopCloser(bytes.NewReader(body)),
			}, nil
		}
		w.publish(readHTTPResponse(ctx, open, false, w.publish))
	}()
}
