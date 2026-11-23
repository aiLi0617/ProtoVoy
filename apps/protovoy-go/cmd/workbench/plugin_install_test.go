package main

import (
	"testing"

	"protovoy.dev/protovoy/internal/agentclient"
	"protovoy.dev/protovoy/internal/plugins"
)

func TestPluginFileSelectionResetsTrustAndIgnoresPreviousAgent(t *testing.T) {
	w := newWorkbench()
	previous, current := &agentclient.Client{}, &agentclient.Client{}
	w.agent.client, w.plugins.installFor = current, current
	w.plugins.installComponent = []byte("old component")
	w.plugins.installUnsigned = true
	w.plugins.installGrants.SetText("network")
	manifest := []byte(`{"id":"fixture","name":"Fixture","version":"1","kind":"transformer"}`)
	w.plugins.filePending = &pluginFileResult{client: previous, data: manifest}
	w.finishPlugins()
	if len(w.plugins.installManifest) != 0 {
		t.Fatal("loaded package files from a previous Agent selection")
	}
	w.plugins.filePending = &pluginFileResult{client: current, data: manifest}
	w.finishPlugins()
	if string(w.plugins.installManifest) != string(manifest) || len(w.plugins.installComponent) != 0 || w.plugins.installUnsigned || w.plugins.installGrants.Text() != "" {
		t.Fatal("new package retained old Component or trust decisions")
	}
	w.plugins.filePending = &pluginFileResult{client: current}
	w.finishPlugins()
	if string(w.plugins.installManifest) != string(manifest) {
		t.Fatal("cancelled file selection discarded current package")
	}
	w.plugins.installComponent = []byte("uploaded component")
	w.plugins.installPending = &pluginInstallResult{client: current, installed: plugins.InstallResult{ID: "fixture"}}
	w.finishPlugins()
	if len(w.plugins.installManifest) != 0 || len(w.plugins.installComponent) != 0 || w.plugins.installUnsigned {
		t.Fatal("completed installation retained upload buffers or unsigned policy")
	}
}
