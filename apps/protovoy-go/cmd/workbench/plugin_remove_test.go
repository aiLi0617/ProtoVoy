package main

import (
	"errors"
	"testing"

	"protovoy.dev/protovoy/internal/agentclient"
	"protovoy.dev/protovoy/internal/plugins"
)

func TestPluginRemovalClearsSelectionOnlyAfterCurrentAgentSuccess(t *testing.T) {
	w := newWorkbench()
	previous, current := &agentclient.Client{}, &agentclient.Client{}
	w.agent.client, w.plugins.loadedFor, w.plugins.selectedFor = current, current, current
	w.plugins.items = []plugins.Installed{{ID: "fixture"}, {ID: "other"}}
	w.plugins.selectedID, w.plugins.selectedKind, w.plugins.output = "fixture", "transformer", "previous output"
	w.plugins.input.SetText("previous input")
	w.plugins.grants.SetText("network")
	w.plugins.allowUnsigned = true
	w.plugins.installMessage = "Installed plugin: fixture. Refresh installed plugins to select it."
	for _, result := range []*pluginRemoveResult{
		{client: previous, result: plugins.RemoveResult{ID: "fixture"}},
		{client: current, err: errors.New("rejected")},
	} {
		w.plugins.removePending = result
		w.finishPlugins()
		if len(w.plugins.items) != 2 || w.plugins.selectedID != "fixture" {
			t.Fatal("applied stale or rejected removal")
		}
	}
	w.plugins.removePending = &pluginRemoveResult{client: current, result: plugins.RemoveResult{ID: "fixture"}}
	w.finishPlugins()
	if len(w.plugins.items) != 1 || w.plugins.items[0].ID != "other" || w.plugins.selectedID != "" || w.plugins.selectedFor != nil ||
		w.plugins.output != "" || w.plugins.input.Text() != "" || w.plugins.grants.Text() != "" || w.plugins.allowUnsigned || w.plugins.installMessage != "" {
		t.Fatal("successful removal retained the package selection or execution data")
	}
}
