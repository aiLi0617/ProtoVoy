//go:build !js

package scheduler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"protovoy.dev/protovoy/internal/project"
)

// Handler requires the host to authorize every request, including reads.
// The authorizer must check both identity and origin for browser mutations.
func (s *Store) Handler(authorize func(*http.Request) bool) http.Handler {
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, status int, value any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(value)
	}
	decode := func(w http.ResponseWriter, r *http.Request, value any) bool {
		r.Body = http.MaxBytesReader(w, r.Body, 384<<10)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(value) != nil || decoder.Decode(new(any)) != io.EOF {
			http.Error(w, "invalid schedule document", http.StatusBadRequest)
			return false
		}
		return true
	}
	mux.HandleFunc("GET /v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		jobs, err := s.List(r.Context())
		if err != nil {
			http.Error(w, "could not read schedules", http.StatusInternalServerError)
			return
		}
		write(w, http.StatusOK, jobs)
	})
	mux.HandleFunc("POST /v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Name            string          `json:"name"`
			IntervalSeconds int             `json:"intervalSeconds"`
			Enabled         bool            `json:"enabled"`
			Request         project.Request `json:"request"`
		}
		if !decode(w, r, &input) {
			return
		}
		// Validate before storage so malformed input is not reported as a database failure.
		if input.IntervalSeconds < 10 || input.IntervalSeconds > 31536000 || len(input.Name) > 1024 {
			http.Error(w, "invalid schedule interval or name", http.StatusBadRequest)
			return
		}
		if _, err := project.CaptureHistoryRequest(input.Request); err != nil {
			http.Error(w, "invalid or credential-bearing schedule request", http.StatusBadRequest)
			return
		}
		job, err := s.Create(r.Context(), input.Name, input.IntervalSeconds, input.Enabled, input.Request, time.Now())
		if err != nil {
			http.Error(w, "could not create schedule", http.StatusInternalServerError)
			return
		}
		write(w, http.StatusCreated, job)
	})
	mux.HandleFunc("PATCH /v1/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Enabled *bool `json:"enabled"`
		}
		if !decode(w, r, &input) {
			return
		}
		if input.Enabled == nil {
			http.Error(w, "enabled is required", http.StatusBadRequest)
			return
		}
		job, err := s.SetEnabled(r.Context(), r.PathValue("id"), *input.Enabled, time.Now())
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "could not update schedule", http.StatusInternalServerError)
			return
		}
		write(w, http.StatusOK, job)
	})
	mux.HandleFunc("DELETE /v1/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		deleted, err := s.Delete(r.Context(), r.PathValue("id"))
		if err != nil {
			http.Error(w, "could not delete schedule", http.StatusInternalServerError)
			return
		}
		if !deleted {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /v1/executions", func(w http.ResponseWriter, r *http.Request) {
		results, err := s.Executions(r.Context())
		if err != nil {
			http.Error(w, "could not read executions", http.StatusInternalServerError)
			return
		}
		write(w, http.StatusOK, results)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if authorize == nil || !authorize(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
