package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"

	"protovoy.dev/protovoy/internal/project"
)

const workspaceFile = "workspace.json"
const workspaceLimit = 8 << 20

func (agent *agentService) workspace(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if agent.history == nil {
		http.Error(w, "workspace storage unavailable", http.StatusServiceUnavailable)
		return
	}
	store := agent.history
	var data []byte
	if r.Method == http.MethodPut {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, workspaceLimit))
		if err != nil {
			http.Error(w, "workspace exceeds 8 MiB or could not be read", http.StatusRequestEntityTooLarge)
			return
		}
		workspace, err := project.DecodeWorkspace(body)
		if err != nil {
			http.Error(w, "invalid workspace", http.StatusBadRequest)
			return
		}
		data, err = project.EncodeWorkspace(workspace)
		if err != nil {
			http.Error(w, "workspace contains invalid or sensitive data", http.StatusBadRequest)
			return
		}
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if r.Method != http.MethodGet && (r.Header.Get("If-Match") != "" || r.Header.Get("If-None-Match") != "") {
		file, err := store.root.Open(workspaceFile)
		exists := err == nil
		var current []byte
		if exists {
			current, err = io.ReadAll(io.LimitReader(file, workspaceLimit+1))
			_ = file.Close()
		}
		if err != nil && !os.IsNotExist(err) || len(current) > workspaceLimit {
			http.Error(w, "could not inspect current workspace", http.StatusInternalServerError)
			return
		}
		match := r.Header.Get("If-Match")
		if match != "" && (!exists || match != "*" && match != workspaceETag(current)) ||
			r.Header.Get("If-None-Match") == "*" && exists {
			http.Error(w, "workspace changed; reload before saving", http.StatusPreconditionFailed)
			return
		}
	}
	switch r.Method {
	case http.MethodGet:
		file, err := store.root.Open(workspaceFile)
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "could not read workspace", http.StatusInternalServerError)
			return
		}
		defer file.Close()
		data, err = io.ReadAll(io.LimitReader(file, workspaceLimit+1))
		if err != nil || len(data) > workspaceLimit {
			http.Error(w, "could not read workspace within 8 MiB", http.StatusInternalServerError)
			return
		}
		value, err := project.DecodeWorkspace(data)
		if err != nil {
			http.Error(w, "stored workspace is invalid", http.StatusInternalServerError)
			return
		}
		if _, err := project.EncodeWorkspace(value); err != nil {
			http.Error(w, "stored workspace contains invalid or sensitive data", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", workspaceETag(data))
		_, _ = w.Write(data)
	case http.MethodPut:
		const pending = "workspace.pending"
		file, err := store.root.OpenFile(pending, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if err == nil {
			_, err = file.Write(data)
			if err == nil {
				err = file.Sync()
			}
			if closeErr := file.Close(); err == nil {
				err = closeErr
			}
		}
		if err == nil {
			err = store.root.Rename(pending, workspaceFile)
		}
		if err != nil {
			_ = store.root.Remove(pending)
			http.Error(w, "could not save workspace", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if err := store.root.Remove(workspaceFile); err != nil && !os.IsNotExist(err) {
			http.Error(w, "could not remove workspace", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func workspaceETag(data []byte) string {
	return fmt.Sprintf("\"%x\"", sha256.Sum256(data))
}
