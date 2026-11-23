package main

import (
	"encoding/json"
	"io"
	"net/http"

	"protovoy.dev/protovoy/internal/secrets"
)

func (agent *agentService) httpSecret(w http.ResponseWriter, r *http.Request) {
	reference := r.PathValue("reference")
	var err error
	switch r.Method {
	case http.MethodPut:
		var input struct {
			Target string `json:"target"`
			Header string `json:"header"`
			Value  string `json:"value"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
			http.Error(w, "expected HTTP credential JSON within 16 KiB", http.StatusBadRequest)
			return
		}
		err = secrets.PutHTTP(reference, input.Target, input.Header, input.Value)
	case http.MethodGet:
		var exists bool
		exists, err = secrets.HTTPExists(reference)
		if err == nil {
			writeJSON(w, http.StatusOK, map[string]bool{"exists": exists})
			return
		}
	case http.MethodDelete:
		err = secrets.DeleteHTTP(reference)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
