package main

import (
	"context"

	"github.com/vivaui/viva/flex"
	"protovoy.dev/protovoy/internal/agentclient"
	"protovoy.dev/protovoy/internal/plugins"
)

type pluginRemoveResult struct {
	client *agentclient.Client
	result plugins.RemoveResult
	err    error
}

func (w *workbench) drawPluginRemoval(f *flex.Frame, id string) {
	if w.plugins.busy || !plugins.ValidID(id) {
		return
	}
	if f.Button(tree.ID(page, "plugin-remove-"+id), "Uninstall "+id).Clicked() {
		client := w.agent.client
		w.plugins.busy = true
		go func() {
			result, err := client.RemovePlugin(context.Background(), id)
			w.mu.Lock()
			w.plugins.removePending = &pluginRemoveResult{client, result, err}
			w.mu.Unlock()
			w.invalidator.Invalidate()
		}()
	}
}

func (w *workbench) finishPluginRemoval() {
	w.mu.Lock()
	result := w.plugins.removePending
	w.plugins.removePending = nil
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
	p := &w.plugins
	p.installMessage = ""
	for i := range p.items {
		if p.items[i].ID == result.result.ID {
			copy(p.items[i:], p.items[i+1:])
			p.items[len(p.items)-1] = plugins.Installed{}
			p.items = p.items[:len(p.items)-1]
			break
		}
	}
	p.message = "Uninstalled plugin: " + result.result.ID
	if result.result.CleanupPending {
		p.message += ". Package file cleanup is pending."
	}
	if p.selectedFor == result.client && p.selectedID == result.result.ID {
		p.selectedID, p.selectedKind, p.selectedFor = "", "", nil
		p.output, p.runMessage, p.allowUnsigned = "", "", false
		p.input.SetText("")
		p.grants.SetText("")
	}
}
