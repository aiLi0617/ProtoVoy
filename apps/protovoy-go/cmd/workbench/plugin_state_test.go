package main

import (
	"errors"
	"testing"

	"protovoy.dev/protovoy/internal/agentclient"
	"protovoy.dev/protovoy/internal/plugins"
)

func TestPluginStateOnlyUpdatesAfterCurrentAgentSuccess(t *testing.T) {
	w := newWorkbench()
	previous, current := &agentclient.Client{}, &agentclient.Client{}
	w.agent.client, w.plugins.loadedFor, w.plugins.selectedFor = current, current, current
	w.plugins.selectedID = "fixture"
	w.plugins.items = []plugins.Installed{{ID: "fixture"}}
	for _, result := range []*pluginStateResult{
		{client: previous, id: "fixture", enabled: false},
		{client: current, id: "fixture", enabled: false, err: errors.New("rejected")},
	} {
		w.plugins.statePending = result
		w.finishPlugins()
		if w.plugins.items[0].Disabled {
			t.Fatal("applied stale or rejected plugin state")
		}
	}
	w.plugins.allowUnsigned, w.plugins.output = true, "old output"
	w.plugins.grants.SetText("network")
	w.plugins.statePending = &pluginStateResult{client: current, id: "fixture", enabled: false}
	w.finishPlugins()
	if !w.plugins.items[0].Disabled || w.plugins.allowUnsigned || w.plugins.grants.Text() != "" || w.plugins.output != "" {
		t.Fatal("state change failed to update inventory and clear execution choices")
	}
}
