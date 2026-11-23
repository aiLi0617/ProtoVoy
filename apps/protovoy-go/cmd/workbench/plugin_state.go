package main

import (
	"context"

	"protovoy.dev/protovoy/internal/agentclient"
)

type pluginStateResult struct {
	client  *agentclient.Client
	id      string
	enabled bool
	err     error
}

func (w *workbench) changePluginState(id string, enabled bool) {
	client := w.agent.client
	if client == nil || w.plugins.busy {
		return
	}
	w.plugins.busy = true
	go func() {
		err := client.SetPluginEnabled(context.Background(), id, enabled)
		w.mu.Lock()
		w.plugins.statePending = &pluginStateResult{client, id, enabled, err}
		w.mu.Unlock()
		w.invalidator.Invalidate()
	}()
}

func (w *workbench) finishPluginState() {
	w.mu.Lock()
	result := w.plugins.statePending
	w.plugins.statePending = nil
	w.mu.Unlock()
	if result == nil {
		return
	}
	w.plugins.busy = false
	if result.client != w.agent.client || result.client != w.plugins.loadedFor {
		return
	}
	if result.err != nil {
		w.plugins.message = result.err.Error()
		return
	}
	for i := range w.plugins.items {
		if w.plugins.items[i].ID == result.id {
			w.plugins.items[i].Disabled = !result.enabled
		}
	}
	w.plugins.message = "Disabled plugin: " + result.id
	if result.enabled {
		w.plugins.message = "Enabled plugin: " + result.id
	}
	if w.plugins.selectedFor == result.client && w.plugins.selectedID == result.id {
		w.plugins.output, w.plugins.runMessage = "", ""
		w.plugins.allowUnsigned = false
		w.plugins.grants.SetText("")
	}
}
