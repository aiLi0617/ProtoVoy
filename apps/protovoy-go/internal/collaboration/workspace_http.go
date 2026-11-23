//go:build !js

package collaboration

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
)

// WorkspaceAccess returns the authenticated actor or an HTTP denial status.
// An edit requires at least the editor role, and reads require at least viewer.
type WorkspaceAccess func(r *http.Request, organization string, edit bool) (actor string, denial int)

func (store *WorkspaceStore) RegisterWorkspaceRoutes(mux *http.ServeMux, access WorkspaceAccess) {
	const path = "/v1/organizations/{organizationId}/workspaces/{workspaceId}"
	for _, route := range []struct {
		pattern string
		edit    bool
		serve   func(http.ResponseWriter, *http.Request, string, string, string)
	}{
		{"GET " + path, false, store.getWorkspace},
		{"PUT " + path, true, store.pushWorkspace},
		{"GET " + path + "/changes", false, store.workspaceChanges},
	} {
		mux.HandleFunc(route.pattern, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			org := r.PathValue("organizationId")
			actor, denial := access(r, org, route.edit)
			if denial != 0 {
				http.Error(w, http.StatusText(denial), denial)
				return
			}
			if actor == "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			route.serve(w, r, org, r.PathValue("workspaceId"), actor)
		})
	}
}

func (store *WorkspaceStore) getWorkspace(w http.ResponseWriter, r *http.Request, org, id, _ string) {
	value, err := store.Get(r.Context(), org, id)
	writeWorkspaceResponse(w, value, err)
}

func (store *WorkspaceStore) workspaceChanges(w http.ResponseWriter, r *http.Request, org, id, _ string) {
	var after int64
	if text := r.URL.Query().Get("afterRevision"); text != "" {
		var err error
		after, err = strconv.ParseInt(text, 10, 64)
		if err != nil || after < 0 {
			http.Error(w, "invalid afterRevision", http.StatusBadRequest)
			return
		}
	}
	changes, err := store.Changes(r.Context(), org, id, after)
	writeWorkspaceResponse(w, changes, err)
}

func (store *WorkspaceStore) pushWorkspace(w http.ResponseWriter, r *http.Request, org, id, actor string) {
	var input struct {
		BaseRevision int64           `json:"baseRevision"`
		Document     json.RawMessage `json:"document"`
		Patch        json.RawMessage `json:"patch"`
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWorkspacePayload))
	var limit *http.MaxBytesError
	switch {
	case errors.As(err, &limit):
		http.Error(w, "workspace update is too large", http.StatusRequestEntityTooLarge)
		return
	case err != nil || json.Unmarshal(data, &input) != nil || bytes.Equal(input.Document, []byte("null")):
		http.Error(w, "invalid workspace update", http.StatusBadRequest)
		return
	}
	if input.Patch == nil {
		input.Patch = json.RawMessage("null")
	}
	value, err := store.Push(r.Context(), org, id, actor, input.BaseRevision, input.Document, input.Patch)
	writeWorkspaceResponse(w, value, err)
}

func writeWorkspaceResponse(w http.ResponseWriter, value any, err error) {
	status := http.StatusOK
	var conflict *Conflict
	switch {
	case errors.As(err, &conflict):
		status = http.StatusConflict
		value = struct {
			Code            string          `json:"code"`
			Message         string          `json:"message"`
			CurrentRevision int64           `json:"currentRevision"`
			CurrentDocument json.RawMessage `json:"currentDocument"`
		}{"revision_conflict", conflict.Error(), conflict.Current.Revision, conflict.Current.Document}
	case errors.Is(err, errInvalidWorkspaceUpdate):
		http.Error(w, errInvalidWorkspaceUpdate.Error(), http.StatusBadRequest)
		return
	case err != nil:
		http.Error(w, "workspace operation failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
