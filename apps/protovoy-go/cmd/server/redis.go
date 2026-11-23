package main

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"protovoy.dev/protovoy/internal/execution"
)

func validateHostedRedisTarget(target, database string) error {
	if target == "" {
		return nil
	}
	if err := validateHostedSocketTarget(target); err != nil {
		return err
	}
	if database != "" {
		if _, err := strconv.ParseUint(database, 10, 64); err != nil {
			return errors.New("Redis database must be a nonnegative integer")
		}
	}
	return nil
}

func loadRedisCA(path string) (string, error) {
	pem, err := execution.ReadCAFile(path)
	if err != nil || len(pem) == 0 {
		return "", errors.New("could not read a nonempty Redis CA file within 1 MiB")
	}
	if !x509.NewCertPool().AppendCertsFromPEM(pem) {
		return "", errors.New("Redis CA file contains no usable certificates")
	}
	return string(pem), nil
}

func redisHandler(origin, target string, options execution.RedisOptions, references ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r, origin) || r.Header.Get("X-ProtoVoy-Preview") != "1" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if target == "" {
			http.Error(w, "Redis endpoint is not configured", http.StatusServiceUnavailable)
			return
		}
		var input execution.RedisPayload
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF || len(input.Commands) == 0 {
			http.Error(w, "invalid Redis request", http.StatusBadRequest)
			return
		}
		if input.ExpectedTarget != "" && input.ExpectedTarget != target {
			http.Error(w, "Redis target mismatch", http.StatusConflict)
			return
		}
		if input.Protocol != 0 && input.Protocol != 2 && input.Protocol != 3 {
			http.Error(w, "select Redis RESP2 or RESP3", http.StatusBadRequest)
			return
		}
		requestOptions := options
		if input.ExpectedPasswordRef == "" && (input.ExpectedUsername != "" || input.ExpectedDatabase != "" || input.ExpectedServerName != "") {
			http.Error(w, "Redis credential assertions require a named binding", http.StatusBadRequest)
			return
		}
		if input.ExpectedPasswordRef != "" {
			if !execution.ValidPasswordReference(input.ExpectedPasswordRef) || len(references) != 1 || input.ExpectedPasswordRef != references[0] || !options.TLS || options.Password == "" {
				http.Error(w, "Redis password reference does not match the configured endpoint", http.StatusBadRequest)
				return
			}
			if input.ExpectedUsername != "" && input.ExpectedUsername != options.Username || input.ExpectedServerName != "" && input.ExpectedServerName != options.ServerName {
				http.Error(w, "Redis credential settings mismatch", http.StatusConflict)
				return
			}
			if input.ExpectedDatabase != "" {
				configured := options.Database
				if configured == "" {
					configured = "0"
				}
				actual, err := strconv.ParseUint(configured, 10, 64)
				expected, expectedErr := strconv.ParseUint(input.ExpectedDatabase, 10, 64)
				if err != nil || expectedErr != nil || actual != expected {
					http.Error(w, "Redis database mismatch", http.StatusConflict)
					return
				}
			}
		}
		requestOptions.Protocol = input.Protocol
		if !execution.ValidRequestTimeout(input.TimeoutMS) {
			http.Error(w, "invalid Redis timeout", http.StatusBadRequest)
			return
		}
		timeout := execution.RequestTimeout(input.TimeoutMS)
		if timeout > execution.RequestTimeout(0) {
			err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(timeout + 5*time.Second))
			if err != nil && !errors.Is(err, http.ErrNotSupported) {
				http.Error(w, "could not prepare Redis execution", http.StatusInternalServerError)
				return
			}
		}
		values, err := execution.ExecuteRedisWithOptions(r.Context(), target, input.Commands, timeout, requestOptions)
		if err != nil {
			if errors.Is(r.Context().Err(), context.Canceled) {
				http.Error(w, "Redis execution canceled", http.StatusServiceUnavailable)
				return
			}
			if errors.Is(err, context.DeadlineExceeded) {
				http.Error(w, "Redis execution timed out", http.StatusGatewayTimeout)
				return
			}
			http.Error(w, "Redis exchange failed", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_ = json.NewEncoder(w).Encode(values)
	}
}
