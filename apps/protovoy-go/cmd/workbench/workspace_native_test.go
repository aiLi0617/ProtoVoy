//go:build !js

package main

import (
	"strings"
	"testing"

	"protovoy.dev/protovoy/internal/project"
)

func TestApplyingEmptyWorkspaceClearsPreviousSelection(t *testing.T) {
	w := newWorkbench()
	w.finishImport(projectImport{value: &project.Project{Name: "Previous", Requests: []project.Request{
		{Name: "Saved", Method: "GET", URL: "http://example.test/"},
	}}})
	w.loadRequest(1, w.collection.project.Requests[0])
	w.collection.page, w.collection.replace = 3, true
	w.applyWorkspace(project.Workspace{Version: 1, Draft: project.Draft{Name: "Fresh", Method: "GET", Target: "http://fresh.test/"}})
	if w.collection.project != nil || w.collection.selected != 0 || w.collection.page != 0 || w.collection.replace || w.draft().Name != "Fresh" {
		t.Fatal("applying workspace retained previous collection state")
	}
}

func TestWorkspaceRestoresDraftAndProtectsNewerSnapshot(t *testing.T) {
	t.Setenv("PROTOVOY_CONFIG_DIR", t.TempDir())
	reopen := func() *workbench {
		w := newWorkbench()
		w.restoreWorkspace()
		return w
	}

	first := reopen()
	first.finishImport(projectImport{value: &project.Project{Name: "Demo", Requests: []project.Request{
		{Name: "Saved name", Method: "POST", URL: "{{endpoint}}", Variables: map[string]string{"endpoint": "https://example.test"}},
	}}})
	first.loadRequest(1, first.collection.project.Requests[0])
	first.variables.scopes = project.VariableScopes{
		Global:       map[string]string{"region": "local"},
		Environments: []project.Environment{{Name: "Test", Variables: map[string]string{"region": "test"}}},
		Active:       "Test",
	}
	first.requestName.SetText("Draft name")
	first.flushWorkspace()
	if first.autosave.notice != "" {
		t.Fatal(first.autosave.notice)
	}

	second := reopen()
	if second.collection.project == nil || second.collection.project.Name != "Demo" ||
		second.collection.selected != 1 || second.collection.baseline.Name != "Saved name" ||
		second.draft().Name != "Draft name" || second.collection.variables["endpoint"] != "https://example.test" ||
		second.variables.scopes.ActiveVariables()["region"] != "test" {
		t.Fatal("restored state lost the selected request, variables or edited draft")
	}
	first.requestName.SetText("Newer draft")
	first.flushWorkspace()
	second.requestName.SetText("Stale window draft")
	second.flushWorkspace()
	if !strings.Contains(second.autosave.notice, "another window") || reopen().draft().Name != "Newer draft" {
		t.Fatal("stale window overwrote the newer workspace")
	}

	first.headers.input.Editor.Insert("Authorization: Bearer example")
	first.flushWorkspace()
	if !strings.Contains(first.autosave.notice, "possible credentials") || reopen().draft().Headers != "" {
		t.Fatal("credentials entered the workspace snapshot")
	}
	first.applyDraft(reopen().draft())
	first.variables.scopes.Environments[0].Variables["api_key"] = "example"
	first.variables.revision++
	first.flushWorkspace()
	if !strings.Contains(first.autosave.notice, "possible credentials") || reopen().variables.scopes.ActiveVariables()["api_key"] != "" {
		t.Fatal("an environment credential entered the workspace snapshot")
	}
}
