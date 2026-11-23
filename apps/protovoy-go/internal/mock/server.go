//go:build !js

package mock

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	maxBodyBytes = 10 << 20
)

type service struct {
	path         string
	requests     atomic.Uint64
	active       atomic.Int64
	mu           sync.Mutex
	file         os.FileInfo
	rules        []Rule
	hits         map[string]uint64
	reloadFailed bool
}

// ValidateFile checks a complete scenario file without starting a listener.
func ValidateFile(path string) error {
	app := &service{path: path, hits: make(map[string]uint64)}
	return app.reload()
}

func NewHandler(path, bind, version string) (http.Handler, error) {
	app := &service{path: path, hits: make(map[string]uint64)}
	if err := app.reload(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return app.handler(bind, version), nil
}

func (app *service) handler(bind, version string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		app.mu.Lock()
		failed := app.reloadFailed
		app.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"service": "protovoy-mock", "bind": bind, "version": version,
			"requestCount": app.requests.Load(), "activeWebsockets": app.active.Load(),
			"rulesReloadFailed": failed,
		})
	})
	mux.HandleFunc("/m1/{project}/{service}/{path...}", app.serve)
	mux.HandleFunc("/m2/{project}/{service}/{operation}", app.serve)
	return mux
}

// reload is called while holding the service lock once requests can arrive.
func (app *service) reload() error {
	if app.path == "" {
		return nil
	}
	file, err := os.Open(app.path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > maxRulesBytes {
		return errors.New("mock rules must be a regular file of at most 8 MiB")
	}
	if app.file != nil && os.SameFile(info, app.file) && info.Size() == app.file.Size() && info.ModTime().Equal(app.file.ModTime()) {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(file, maxRulesBytes+1))
	if err != nil || len(data) > maxRulesBytes {
		return errors.New("could not read mock rules within 8 MiB")
	}
	rules, err := Parse(data)
	if err != nil {
		return err
	}
	hits := make(map[string]uint64, len(rules))
	for _, rule := range rules {
		hits[rule.ID] = app.hits[rule.ID]
	}
	app.file, app.rules, app.hits = info, rules, hits
	return nil
}

func (app *service) selectRule(request Request) (*Rule, []string, uint64) {
	app.mu.Lock()
	defer app.mu.Unlock()
	// Keep serving the last valid snapshot during incomplete writes.
	app.reloadFailed = app.reload() != nil
	rule, conflicts := Select(app.rules, request)
	if rule == nil {
		return nil, conflicts, 0
	}
	app.hits[rule.ID]++
	return rule, nil, app.hits[rule.ID]
}

func (app *service) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	app.requests.Add(1)
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			http.Error(w, "mock request body exceeds 10 MiB", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "could not read mock request body", http.StatusBadRequest)
		}
		return
	}
	query := r.URL.Query()
	request := Request{
		ProjectKey: r.PathValue("project"), ServiceKey: r.PathValue("service"),
		Method: r.Method, OperationID: r.PathValue("operation"),
		ResponseID: query.Get("protovoyResponseId"), ScenarioID: query.Get("protovoyScenarioId"),
		Query: query, Headers: r.Header, Cookies: make(map[string]string), Body: string(body),
	}
	request.ClientIP, _, _ = net.SplitHostPort(r.RemoteAddr)
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		request.Method = "WS"
	}
	for _, cookie := range r.Cookies() {
		request.Cookies[cookie.Name] = cookie.Value
	}
	if request.OperationID == "" {
		path := r.PathValue("path")
		request.Path, request.OperationID = &path, query.Get("protovoyApiId")
	}
	for _, selector := range []string{"protovoyApiId", "protovoyResponseId", "protovoyScenarioId"} {
		query.Del(selector)
	}
	rule, conflicts, hits := app.selectRule(request)
	switch {
	case len(conflicts) != 0:
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "multiple_mock_responses_matched", "scenarioIds": conflicts,
			"hint": "Specify protovoyApiId or protovoyScenarioId",
		})
		return
	case rule == nil:
		http.Error(w, "No ProtoVoy Mock response matched", http.StatusNotFound)
		return
	}
	if request.Method == "WS" {
		app.serveWebSocket(w, r, rule)
		return
	}
	if rule.DelayMS > 0 {
		timer := time.NewTimer(time.Duration(rule.DelayMS) * time.Millisecond)
		defer timer.Stop()
		select {
		case <-r.Context().Done():
			return
		case <-timer.C:
		}
	}
	if rule.ErrorEvery > 0 && hits%rule.ErrorEvery == 0 {
		w.Header().Set("X-ProtoVoy-Mock-Injected", "true")
		http.Error(w, "Injected mock failure", http.StatusServiceUnavailable)
		return
	}
	for name, value := range rule.Headers {
		switch strings.ToLower(name) {
		case "content-length", "transfer-encoding", "connection", "upgrade", "trailer":
			continue
		}
		w.Header().Set(name, value)
	}
	w.Header().Set("X-ProtoVoy-Mock-Scenario", rule.ID)
	w.WriteHeader(rule.Status)
	_, _ = io.WriteString(w, rule.Body)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
