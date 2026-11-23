package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/flex"
	"protovoy.dev/protovoy/internal/agentclient"
	"protovoy.dev/protovoy/internal/plugins"
)

var (
	pluginInstallOpenID     = tree.ID(page, "plugin-install-open")
	pluginManifestID        = tree.ID(page, "plugin-manifest")
	pluginComponentID       = tree.ID(page, "plugin-component")
	pluginInstallGrantsID   = tree.ID(page, "plugin-install-grants")
	pluginInstallUnsignedID = tree.ID(page, "plugin-install-unsigned")
	pluginInstallID         = tree.ID(page, "plugin-install")
)

type pluginFileResult struct {
	client    *agentclient.Client
	component bool
	data      []byte
	err       error
}

type pluginInstallResult struct {
	client    *agentclient.Client
	installed plugins.InstallResult
	update    bool
	err       error
}

func (w *workbench) drawPluginInstall(f *flex.Frame, client *agentclient.Client) {
	p := &w.plugins
	if p.installFor != client {
		p.installFor = client
		p.installManifest, p.installComponent = nil, nil
		p.installUnsigned, p.installMessage = false, ""
		p.installGrants.SetText("")
	}
	if f.Button(pluginInstallOpenID, "Install plugin package").Clicked() {
		p.installOpen = !p.installOpen
	}
	if !p.installOpen {
		return
	}
	f.Wrap("Select manifest.json and plugin.wasm from your computer. The package is stored on the paired Local Agent machine.").Muted()
	if !p.busy && !p.picking && f.Button(pluginManifestID, "Choose plugin manifest").Clicked() {
		w.choosePluginFile(false)
	}
	if manifest, err := plugins.ReadManifest(p.installManifest); err == nil {
		updateID := ""
		if p.loadedFor == client {
			for _, item := range p.items {
				if item.ID == manifest.ID && item.Manifest != nil && item.Error == "" {
					if !client.SupportsPluginUpdates() {
						f.Wrap("Update Local Agent to replace this installed package.")
						return
					}
					updateID = item.ID
					f.Wrap("Installed version: " + item.Manifest.Version)
					break
				}
			}
		}
		f.Wrap(fmt.Sprintf("Package: %s (%s) · %s · %s", manifest.Name, manifest.ID, manifest.Version, manifest.Kind))
		f.Wrap("Declared permissions: " + strings.Join(manifest.Permissions, ", ")).Muted()
		if !p.busy && !p.picking && f.Button(pluginComponentID, "Choose plugin Component").Clicked() {
			w.choosePluginFile(true)
		}
		if len(p.installComponent) > 0 {
			f.Wrap(fmt.Sprintf("Component selected: %d bytes", len(p.installComponent)))
		}
		f.Field(&p.installGrants, pluginInstallGrantsID, "Installation permission grants", "Installation grants, comma-separated")
		if !p.busy && !p.picking {
			label := "Allow unsigned development package for installation"
			if p.installUnsigned {
				label = "Unsigned installation allowed · require signature instead"
			}
			if f.Button(pluginInstallUnsignedID, label).Clicked() {
				p.installUnsigned = !p.installUnsigned
			}
			action := "Install on Local Agent"
			if updateID != "" {
				action = "Update on Local Agent"
			}
			if len(p.installComponent) > 0 && f.Button(pluginInstallID, action).Clicked() {
				w.installSelectedPlugin(updateID)
			}
		}
	}
	f.Wrap(p.installMessage)
}

func (w *workbench) choosePluginFile(component bool) {
	client := w.agent.client
	w.plugins.picking = true
	options := app.OpenFileOptions{Title: "Choose plugin manifest", Filters: []app.FileFilter{{Name: "JSON manifest", Pattern: "*.json"}}}
	limit := plugins.ManifestMaxBytes
	if component {
		options = app.OpenFileOptions{Title: "Choose plugin Component", Filters: []app.FileFilter{{Name: "WebAssembly Component", Pattern: "*.wasm"}}}
		limit = plugins.ComponentMaxBytes
	}
	chooseLocalFile(options, limit, func(data []byte, err error) {
		w.mu.Lock()
		w.plugins.filePending = &pluginFileResult{client, component, data, err}
		w.mu.Unlock()
		w.invalidator.Invalidate()
	})
}

func (w *workbench) installSelectedPlugin(updateID string) {
	p, client := &w.plugins, w.agent.client
	if p.busy || p.picking || client == nil || p.installFor != client {
		return
	}
	permissions := []string{}
	for _, permission := range strings.Split(p.installGrants.Text(), ",") {
		if permission = strings.TrimSpace(permission); permission != "" {
			permissions = append(permissions, permission)
		}
	}
	input := plugins.InstallRequest{Manifest: p.installManifest, Component: p.installComponent, AllowUnsigned: p.installUnsigned, Permissions: permissions}
	p.busy, p.installMessage = true, "Installing plugin…"
	if updateID != "" {
		p.installMessage = "Updating plugin…"
	}
	go func() {
		var installed plugins.InstallResult
		var err error
		if updateID != "" {
			installed, err = client.UpdatePlugin(context.Background(), updateID, input)
		} else {
			installed, err = client.InstallPlugin(context.Background(), input)
		}
		w.mu.Lock()
		w.plugins.installPending = &pluginInstallResult{client, installed, updateID != "", err}
		w.mu.Unlock()
		w.invalidator.Invalidate()
	}()
}

func (w *workbench) finishPluginInstall() {
	w.mu.Lock()
	file, installed := w.plugins.filePending, w.plugins.installPending
	w.plugins.filePending, w.plugins.installPending = nil, nil
	w.mu.Unlock()
	p := &w.plugins
	if file != nil {
		p.picking = false
		if file.client == w.agent.client && file.client == p.installFor {
			switch {
			case file.err != nil:
				p.installMessage = file.err.Error()
			case file.data == nil:
				// Cancellation leaves the current package selection intact.
			case file.component:
				p.installComponent, p.installMessage = file.data, ""
			default:
				if _, err := plugins.ReadManifest(file.data); err != nil {
					p.installMessage = err.Error()
				} else {
					p.installManifest, p.installComponent = file.data, nil
					p.installUnsigned, p.installMessage = false, ""
					p.installGrants.SetText("")
				}
			}
		}
	}
	if installed == nil {
		return
	}
	p.busy = false
	if installed.client != w.agent.client || installed.client != p.installFor {
		return
	}
	if installed.err != nil {
		p.installMessage = installed.err.Error()
		return
	}
	p.installMessage = "Installed plugin: " + installed.installed.ID + ". Refresh installed plugins to select it."
	if installed.update {
		p.installMessage = "Updated plugin: " + installed.installed.ID + "."
		if p.selectedID == installed.installed.ID && p.selectedFor == installed.client {
			p.selectedID, p.selectedKind, p.selectedFor = "", "", nil
			p.allowUnsigned = false
			p.grants.SetText("")
			p.output, p.runMessage = "", ""
		}
		w.refreshPlugins(installed.client)
	}
	p.installManifest, p.installComponent = nil, nil
	p.installUnsigned = false
	p.installGrants.SetText("")
}
