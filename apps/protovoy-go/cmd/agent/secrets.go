package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"protovoy.dev/protovoy/internal/secrets"
)

func (agent *agentService) secret(w http.ResponseWriter, r *http.Request) {
	reference := r.PathValue("reference")
	var err error
	switch r.Method {
	case http.MethodPut:
		var input struct {
			Value string `json:"value"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
			http.Error(w, "expected a secret value JSON object within 16 KiB", http.StatusBadRequest)
			return
		}
		err = secrets.Put(reference, input.Value)
	case http.MethodGet:
		_, err = secrets.Resolve(reference)
		if errors.Is(err, secrets.ErrNotFound) {
			writeJSON(w, http.StatusOK, map[string]bool{"exists": false})
			return
		}
		if err == nil {
			writeJSON(w, http.StatusOK, map[string]bool{"exists": true})
			return
		}
	case http.MethodDelete:
		err = secrets.Delete(reference)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
