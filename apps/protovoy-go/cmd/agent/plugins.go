package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"unicode/utf8"

	"protovoy.dev/protovoy/internal/plugins"
)

func (agent *agentService) removePlugin(w http.ResponseWriter, r *http.Request) {
	if !plugins.ValidID(r.PathValue("id")) {
		http.Error(w, "invalid plugin ID", http.StatusBadRequest)
		return
	}
	if !agent.pluginMu.TryLock() {
		http.Error(w, "another plugin operation is in progress", http.StatusTooManyRequests)
		return
	}
	defer agent.pluginMu.Unlock()
	directory, err := plugins.StoreDirectory(agent.pluginDirectory)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	result, err := plugins.RemoveInstalled(directory, r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (agent *agentService) setPluginState(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF || input.Enabled == nil || !plugins.ValidID(r.PathValue("id")) {
		http.Error(w, "expected a plugin ID and enabled boolean", http.StatusBadRequest)
		return
	}
	if !agent.pluginMu.TryLock() {
		http.Error(w, "another plugin operation is in progress", http.StatusTooManyRequests)
		return
	}
	defer agent.pluginMu.Unlock()
	directory, err := plugins.StoreDirectory(agent.pluginDirectory)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := plugins.SetEnabled(directory, r.PathValue("id"), *input.Enabled); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, plugins.StateResult{ID: r.PathValue("id"), Enabled: *input.Enabled})
}

func (agent *agentService) pluginTrust(unsigned bool, permissions []string) (plugins.Trust, error) {
	if len(permissions) > 4 {
		return plugins.Trust{}, errors.New("at most four plugin permission grants are allowed")
	}
	trust := plugins.Trust{Keys: agent.pluginKeys, AllowUnsigned: unsigned, Permissions: map[string]bool{}}
	for _, permission := range permissions {
		switch permission {
		case "network", "filesystem_read", "filesystem_write", "secrets_read":
			trust.Permissions[permission] = true
		default:
			return plugins.Trust{}, errors.New("unknown plugin permission")
		}
	}
	return trust, nil
}

func (agent *agentService) installPlugin(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id != "" && !plugins.ValidID(id) {
		http.Error(w, "invalid plugin ID", http.StatusBadRequest)
		return
	}
	// Reserve before decoding uploads to bound concurrent package buffers.
	if !agent.pluginMu.TryLock() {
		http.Error(w, "another plugin operation is in progress", http.StatusTooManyRequests)
		return
	}
	defer agent.pluginMu.Unlock()
	var input plugins.InstallRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, plugins.InstallMessageMaxBytes))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF || len(input.Manifest) > plugins.ManifestMaxBytes || len(input.Component) > plugins.ComponentMaxBytes {
		http.Error(w, "expected plugin package JSON with a manifest within 64 KiB and Component within 32 MiB", http.StatusBadRequest)
		return
	}
	trust, err := agent.pluginTrust(input.AllowUnsigned, input.Permissions)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	directory, err := plugins.StoreDirectory(agent.pluginDirectory)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var pack plugins.Package
	status := http.StatusCreated
	if id == "" {
		pack, err = plugins.InstallPackageBytes(directory, input.Manifest, input.Component, trust)
	} else {
		pack, err = plugins.UpdatePackageBytes(directory, id, input.Manifest, input.Component, trust)
		status = http.StatusOK
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, status, plugins.InstallResult{ID: pack.Manifest.ID, SHA256: pack.SHA256, SignatureVerified: pack.SignatureVerified})
}

func (agent *agentService) runPlugin(w http.ResponseWriter, r *http.Request) {
	if !plugins.ValidID(r.PathValue("id")) {
		http.Error(w, "invalid plugin ID", http.StatusBadRequest)
		return
	}
	var input plugins.RunRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, plugins.MessageMaxBytes))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF || len(input.Input) > 1<<20 || !utf8.ValidString(input.Input) || len(input.Permissions) > 4 {
		http.Error(w, "expected plugin input JSON with at most 1 MiB of UTF-8 input and four permission grants", http.StatusBadRequest)
		return
	}
	trust, err := agent.pluginTrust(input.AllowUnsigned, input.Permissions)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !agent.pluginMu.TryLock() {
		http.Error(w, "another plugin is running", http.StatusTooManyRequests)
		return
	}
	defer agent.pluginMu.Unlock()
	directory, err := plugins.StoreDirectory(agent.pluginDirectory)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	output, err := plugins.ExecuteInstalledSelection(r.Context(), directory, r.PathValue("id"), trust, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, plugins.RunResult{Output: output})
}

func (agent *agentService) listPlugins(w http.ResponseWriter, r *http.Request) {
	directory, err := plugins.StoreDirectory(agent.pluginDirectory)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items, err := plugins.Inventory(directory)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, items)
}
