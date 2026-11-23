//go:build !js

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"protovoy.dev/protovoy/internal/project"
)

func TestSchedulePanelCreatesDisabledSavedSnapshot(t *testing.T) {
	created := make(chan struct {
		Name     string          `json:"name"`
		Interval int             `json:"intervalSeconds"`
		Enabled  bool            `json:"enabled"`
		Request  project.Request `json:"request"`
	}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "operator" || password != "fixture" {
			t.Error("native credentials were not supplied")
		}
		if r.Method == "POST" {
			var input struct {
				Name     string          `json:"name"`
				Interval int             `json:"intervalSeconds"`
				Enabled  bool            `json:"enabled"`
				Request  project.Request `json:"request"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Error(err)
			}
			created <- input
			w.WriteHeader(201)
			w.Write([]byte(`{"id":"fixture"}`))
			return
		}
		w.Write([]byte(`[]`))
	}))
	defer server.Close()
	w := newWorkbench()
	w.schedules.origin.SetText(server.URL)
	w.schedules.user.SetText("operator")
	w.schedules.password.SetText("fixture")
	w.connectSchedules()
	if w.schedules.password.Text() != "" {
		t.Fatal("password retained in field")
	}
	waitScheduleAction(t, w)
	if w.schedules.client == nil {
		t.Fatal(w.schedules.status)
	}
	w.collection.project = &project.Project{CollectionVariables: map[string]string{"collection": "saved", "override": "collection"}, Requests: []project.Request{{Name: "Saved fixture", Method: "GET", URL: "https://example.test", Variables: map[string]string{"override": "request"}}}}
	w.collection.selected = 1
	w.schedules.interval.SetText("60")
	w.createSchedule()
	waitScheduleAction(t, w)
	select {
	case input := <-created:
		if input.Enabled || input.Interval != 60 || input.Name != "Saved fixture" || input.Request.Variables["collection"] != "saved" || input.Request.Variables["override"] != "request" {
			t.Fatalf("unexpected snapshot: %+v", input)
		}
	default:
		t.Fatal("no schedule created")
	}
	if _, exists := w.collection.project.Requests[0].Variables["collection"]; exists {
		t.Fatal("saved request was mutated")
	}
	w.schedules.interval.SetText("9")
	w.createSchedule()
	if w.cancel != nil {
		t.Fatal("invalid interval started operation")
	}
}

func waitScheduleAction(t *testing.T, w *workbench) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for w.cancel != nil {
		select {
		case <-deadline:
			w.cancel()
			t.Fatal("schedule action did not finish")
		case <-ticker.C:
			w.finishScheduleAction()
		}
	}
}
