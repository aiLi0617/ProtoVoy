package main

import (
	"testing"

	"protovoy.dev/protovoy/internal/agentclient"
	"protovoy.dev/protovoy/internal/plugins"
)

func TestPluginInventoryDiscardsResultsFromPreviousAgent(t *testing.T) {
	w := newWorkbench()
	previous, current := &agentclient.Client{}, &agentclient.Client{}
	w.agent.client = current
	w.plugins.busy = true
	w.plugins.pending = &pluginInventoryResult{client: previous, items: []plugins.Installed{{ID: "previous-machine"}}}
	w.finishPlugins()
	if w.plugins.busy || w.plugins.loadedFor != nil || len(w.plugins.items) != 0 {
		t.Fatal("applied inventory from previous Agent")
	}
	w.plugins.pending = &pluginInventoryResult{client: current, items: []plugins.Installed{{ID: "current-machine"}}}
	w.finishPlugins()
	if w.plugins.loadedFor != current || len(w.plugins.items) != 1 || w.plugins.items[0].ID != "current-machine" {
		t.Fatal("discarded inventory from current Agent")
	}
}

func TestPluginOutputDoesNotCrossAgentsOrSelections(t *testing.T) {
	w := newWorkbench()
	previous, current := &agentclient.Client{}, &agentclient.Client{}
	w.agent.client = current
	w.plugins.selectedFor, w.plugins.selectedID = current, "selected"
	for _, result := range []*pluginRunResult{
		{client: previous, id: "selected", output: "previous-agent"},
		{client: current, id: "other", output: "previous-selection"},
	} {
		w.plugins.runPending = result
		w.finishPlugins()
		if w.plugins.output != "" {
			t.Fatal("applied output from previous Agent or plugin selection")
		}
	}
	w.plugins.runPending = &pluginRunResult{client: current, id: "selected", output: "current-output"}
	w.finishPlugins()
	if w.plugins.output != "current-output" || w.plugins.runMessage != "Plugin completed" {
		t.Fatal("discarded output from current plugin selection")
	}
}
