package main

import (
	"context"
	"maps"

	"protovoy.dev/protovoy/internal/agentclient"
	"protovoy.dev/protovoy/internal/project"
)

func (w *workbench) sendAgentScripts() {
	if w.webSocket || w.sse || w.graphql || w.rpc.protocol != "" || w.datagram != "" {
		w.result = result{status: "Request scripts require HTTP", done: true}
		return
	}
	pre, err := parseRequestScripts(w.preScripts.input.Editor.String())
	var post []string
	if err == nil {
		post, err = parseRequestScripts(w.postScripts.input.Editor.String())
	}
	if err == nil {
		err = (project.Request{PreScripts: pre, PostScripts: post,
			HTTPSecretReference: w.requestSecret.Text(), HTTPSecretHeader: w.requestSecretHeader.Text()}).ValidateScripts()
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
	variables := make(map[string]string)
	for _, layer := range w.variableLayers() {
		maps.Copy(variables, layer)
	}
	client := w.agent.client
	ctx, cancel := context.WithCancel(w.agentExecutionContext())
	w.cancel = cancel
	w.result = result{status: "Sending scripted request via Local Agent…"}
	go func() {
		defer cancel()
		w.publish(readAgentHTTPResult(ctx, false, w.publish, func(update func(agentclient.Response)) (agentclient.Response, error) {
			return client.RunHTTPWithScripts(ctx, target, payload, pre, post, variables, update)
		}))
	}()
}
