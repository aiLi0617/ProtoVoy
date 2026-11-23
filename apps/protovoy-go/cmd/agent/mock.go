package main

import (
	"context"
	"io"
	"net/http"
	"time"

	"protovoy.dev/protovoy/internal/mock"
)

func (agent *agentService) manageMock(w http.ResponseWriter, r *http.Request) {
	var rules []byte
	if r.Method == http.MethodPost {
		var err error
		rules, err = io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<20))
		if err != nil {
			http.Error(w, "could not read Mock rules within 8 MiB", http.StatusRequestEntityTooLarge)
			return
		}
		if _, err := mock.Parse(rules); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	agent.mockMu.Lock()
	defer agent.mockMu.Unlock()
	switch r.Method {
	case http.MethodPost:
		if agent.mockRuntime != nil {
			http.Error(w, "stop the running Mock before starting another", http.StatusConflict)
			return
		}
		runtime, err := mock.Start(context.Background(), "127.0.0.1:0", agentVersion, rules)
		if err != nil {
			http.Error(w, "could not start Mock listener", http.StatusInternalServerError)
			return
		}
		agent.mockRuntime = runtime
	case http.MethodDelete:
		if agent.mockRuntime != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
			defer cancel()
			if err := agent.mockRuntime.Close(ctx); err != nil {
				http.Error(w, "Mock shutdown has not completed", http.StatusServiceUnavailable)
				return
			}
			agent.mockRuntime = nil
		}
	}
	address := ""
	if agent.mockRuntime != nil {
		address = agent.mockRuntime.Address
	}
	writeJSON(w, http.StatusOK, map[string]any{"running": address != "", "address": address})
}
