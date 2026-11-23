package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/vivaui/viva/flex"
	"protovoy.dev/protovoy/internal/agentclient"
	"protovoy.dev/protovoy/internal/plugins"
	"protovoy.dev/protovoy/internal/project"
)

var (
	pluginRefreshID  = tree.ID(page, "plugin-refresh")
	pluginInputID    = tree.ID(page, "plugin-input")
	pluginGrantsID   = tree.ID(page, "plugin-grants")
	pluginUnsignedID = tree.ID(page, "plugin-unsigned")
	pluginRunID      = tree.ID(page, "plugin-run")
	pluginImportID   = tree.ID(page, "plugin-import")
)

type pluginPanel struct {
	busy                                      bool
	loadedFor                                 *agentclient.Client
	items                                     []plugins.Installed
	message                                   string
	pending                                   *pluginInventoryResult
	selectedID, selectedKind, selectedVersion string
	selectedRevision                          string
	selectedFor                               *agentclient.Client
	input, grants                             flex.Field
	allowUnsigned                             bool
	output, runMessage                        string
	runPending                                *pluginRunResult
	installOpen, picking                      bool
	installFor                                *agentclient.Client
	installManifest, installComponent         []byte
	installGrants                             flex.Field
	installUnsigned                           bool
	installMessage                            string
	filePending                               *pluginFileResult
	installPending                            *pluginInstallResult
	statePending                              *pluginStateResult
	removePending                             *pluginRemoveResult
}

type pluginRunResult struct {
	client     *agentclient.Client
	id, output string
	err        error
}

type pluginInventoryResult struct {
	client *agentclient.Client
	items  []plugins.Installed
	err    error
}

func (w *workbench) drawPlugins(f *flex.Frame) {
	client := w.agent.client
	if client == nil || !client.SupportsPlugins() {
		return
	}
	panel := &w.plugins
	w.drawPluginInstall(f, client)
	f.Text("Installed plugins")
	f.Wrap("Installed on the Local Agent machine. Listing does not approve a plugin for execution.").Muted()
	if !panel.busy && f.Button(pluginRefreshID, "Refresh installed plugins").Clicked() {
		w.refreshPlugins(client)
	}
	if panel.loadedFor != client {
		return
	}
	f.Wrap(panel.message)
	for _, item := range panel.items {
		if item.Error != "" {
			f.Wrap(item.ID + ": " + item.Error)
			w.drawPluginRemoval(f, item.ID)
			continue
		}
		if item.Manifest == nil {
			continue
		}
		manifest := item.Manifest
		f.Wrap(fmt.Sprintf("%s (%s) · %s · %s", manifest.Name, item.ID, manifest.Version, manifest.Kind))
		w.drawPluginRemoval(f, item.ID)
		stateLabel, action := "Enabled", "Disable "
		if item.Disabled {
			stateLabel, action = "Disabled", "Enable "
		}
		f.Wrap("Plugin state: " + stateLabel)
		if !panel.busy && f.Button(tree.ID(page, "plugin-state-"+item.ID), action+manifest.Name).Clicked() {
			w.changePluginState(item.ID, item.Disabled)
		}
		if len(manifest.Permissions) > 0 {
			f.Wrap("Declared permissions: " + strings.Join(manifest.Permissions, ", ")).Muted()
		}
		if !panel.busy && f.Button(tree.ID(page, "plugin-select-"+item.ID), "Select "+manifest.Name).Clicked() {
			panel.selectedID, panel.selectedKind, panel.selectedFor = item.ID, manifest.Kind, client
			panel.selectedVersion = manifest.Version
			panel.selectedRevision = item.Revision
			panel.allowUnsigned = false
			panel.grants.SetText("")
			panel.output, panel.runMessage = "", ""
		}
	}
	if panel.selectedFor != client || panel.selectedID == "" {
		return
	}
	f.Wrap("Run plugin: " + panel.selectedID)
	for _, item := range panel.items {
		if item.ID == panel.selectedID && item.Disabled {
			f.Wrap("Enable the selected plugin before running it.")
			return
		}
	}
	f.Field(&panel.input, pluginInputID, "Plugin input", "Plugin string input")
	f.Field(&panel.grants, pluginGrantsID, "Permission grants", "Comma-separated permission grants")
	f.Wrap("Grants must name each declared permission. The current sandbox exposes no host capability imports.").Muted()
	if !panel.busy {
		label := "Allow unsigned development plugin"
		if panel.allowUnsigned {
			label = "Unsigned development plugin allowed · require signature instead"
		}
		if f.Button(pluginUnsignedID, label).Clicked() {
			panel.allowUnsigned = !panel.allowUnsigned
		}
		if f.Button(pluginRunID, "Run selected plugin").Clicked() {
			w.runSelectedPlugin()
		}
	}
	f.Wrap(panel.runMessage)
	if panel.output != "" {
		f.Wrap(panel.output)
		if panel.selectedKind == "importer" && !panel.busy && f.Button(pluginImportID, "Import plugin result").Clicked() {
			w.importPluginResult()
		}
	}
}

func (w *workbench) importPluginResult() {
	panel := &w.plugins
	if panel.busy || panel.selectedKind != "importer" || panel.output == "" ||
		panel.selectedFor == nil || panel.selectedFor != w.agent.client || w.collection.picking {
		return
	}
	if !w.canLeaveDraft() {
		panel.runMessage = w.collection.message
		return
	}
	if w.collection.modified {
		panel.runMessage = "Export project changes before importing a plugin result."
		return
	}
	imported, _, err := project.ImportPluginOutput(panel.output)
	if err != nil {
		panel.runMessage = err.Error()
		return
	}
	revision := w.collection.revision
	w.finishImport(projectImport{value: &imported})
	if w.collection.revision == revision {
		panel.runMessage = w.collection.message
		return
	}
	panel.output, panel.runMessage = "", ""
	w.agent.open = false
}

func (w *workbench) refreshPlugins(client *agentclient.Client) {
	w.plugins.busy = true
	go func() {
		items, err := client.Plugins(context.Background())
		w.mu.Lock()
		w.plugins.pending = &pluginInventoryResult{client, items, err}
		w.mu.Unlock()
		w.invalidator.Invalidate()
	}()
}

func (w *workbench) runSelectedPlugin() {
	panel, client := &w.plugins, w.agent.client
	if panel.busy || client == nil || panel.selectedFor != client || panel.selectedID == "" {
		return
	}
	permissions := []string{}
	for _, permission := range strings.Split(panel.grants.Text(), ",") {
		if permission = strings.TrimSpace(permission); permission != "" {
			permissions = append(permissions, permission)
		}
	}
	id := panel.selectedID
	input := plugins.RunRequest{Kind: panel.selectedKind, Input: panel.input.Text(), AllowUnsigned: panel.allowUnsigned, Permissions: permissions}
	if client.SupportsPluginUpdates() {
		input.ExpectedVersion = panel.selectedVersion
		input.ExpectedRevision = panel.selectedRevision
	}
	panel.busy = true
	panel.output, panel.runMessage = "", "Running plugin…"
	go func() {
		output, err := client.RunPlugin(context.Background(), id, input)
		w.mu.Lock()
		w.plugins.runPending = &pluginRunResult{client, id, output, err}
		w.mu.Unlock()
		w.invalidator.Invalidate()
	}()
}

func (w *workbench) finishPluginRun() {
	w.mu.Lock()
	result := w.plugins.runPending
	w.plugins.runPending = nil
	w.mu.Unlock()
	if result == nil {
		return
	}
	w.plugins.busy = false
	if result.client != w.agent.client || result.id != w.plugins.selectedID || result.client != w.plugins.selectedFor {
		return
	}
	w.plugins.output = result.output
	w.plugins.runMessage = "Plugin completed"
	if result.err != nil {
		w.plugins.output = ""
		w.plugins.runMessage = result.err.Error()
	}
}

func (w *workbench) finishPlugins() {
	w.finishPluginRemoval()
	w.finishPluginState()
	w.finishPluginInstall()
	w.finishPluginRun()
	w.mu.Lock()
	result := w.plugins.pending
	w.plugins.pending = nil
	w.mu.Unlock()
	if result == nil {
		return
	}
	w.plugins.busy = false
	if result.client != w.agent.client {
		return
	}
	w.plugins.loadedFor = result.client
	w.plugins.items = result.items
	w.plugins.message = fmt.Sprintf("%d installed plugin(s)", len(result.items))
	if result.err != nil {
		w.plugins.items = nil
		w.plugins.message = result.err.Error()
	}
}
