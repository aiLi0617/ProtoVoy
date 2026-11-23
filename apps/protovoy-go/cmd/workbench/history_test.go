package main

import (
	"bytes"
	"net/http"
	"testing"

	"protovoy.dev/protovoy/internal/agentclient"
	"protovoy.dev/protovoy/internal/project"
)

func TestHistorySnapshotKeepsOldVersionAndSkipsCredentials(t *testing.T) {
	w := newWorkbench()
	old := project.Request{Name: "Original", URL: "http://localhost:8080/old", Method: http.MethodGet,
		Variables: map[string]string{"region": "west"}}
	current := old
	current.URL = "http://localhost:8080/new"
	id := w.captureHistoryRequest(old)
	if id == "" || w.captureHistoryRequest(old) != id || w.captureHistoryRequest(current) == id || len(w.historySnapshots) != 2 {
		t.Fatal("snapshot IDs did not distinguish saved versions")
	}
	old.Headers = map[string]string{"Authorization": "fixture"}
	if id := w.captureHistoryRequest(old); id != "" || len(w.historySnapshots) != 2 {
		t.Fatal("captured a request with authorization")
	}
	data, err := project.EncodeWorkspace(project.Workspace{Draft: w.draft(), HistorySnapshots: w.historySnapshots, HistorySnapshotsEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := project.DecodeWorkspace(data)
	if err != nil || !decoded.HistorySnapshotsEnabled {
		t.Fatalf("could not restore snapshot index: %v", err)
	}
	w.historySnapshots = decoded.HistorySnapshots
	w.openHistorySnapshot(*w.historySnapshot(id))
	if w.target.Text() != "http://localhost:8080/old" || w.collection.variables["region"] != "west" || w.collection.selected != 0 {
		t.Fatal("historical request did not open as a separate draft")
	}
}

func TestHistoryLinkPersistsOnlyInLocalWorkspace(t *testing.T) {
	w := newWorkbench()
	w.agent.client = &agentclient.Client{}
	w.target.SetText("http://localhost:8080/")
	w.saveDraft()
	if w.collection.selected != 1 {
		t.Fatal("request was not saved")
	}
	w.target.SetText("http://localhost:8080/unsaved")
	w.agentExecutionContext()
	request := &w.collection.project.Requests[0]
	if request.LocalID != "" {
		t.Fatal("linked an execution from an unsaved draft")
	}
	w.discardDraft()
	w.agentExecutionContext()
	if request.LocalID == "" || w.historyRequestIndex(request.LocalID) != 0 {
		t.Fatal("saved request did not get a stable history link")
	}
	encoded, err := project.EncodeWorkspace(project.Workspace{Project: w.collection.project, Draft: w.draft(), Selected: 1})
	if err != nil {
		t.Fatal(err)
	}
	local, err := project.DecodeWorkspace(encoded)
	if err != nil || local.Project.Requests[0].LocalID != request.LocalID {
		t.Fatalf("history link did not survive workspace restore: %v", err)
	}
	portable, err := project.Encode(*w.collection.project)
	if err != nil || bytes.Contains(portable, []byte(request.LocalID)) {
		t.Fatalf("local history link escaped in project export: %v", err)
	}
}
