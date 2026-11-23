package main

import (
	"testing"

	"protovoy.dev/protovoy/internal/agentclient"
	"protovoy.dev/protovoy/internal/project"
)

func TestAgentWorkspaceLoadKeepsEditsMadeWhileWaiting(t *testing.T) {
	w := newWorkbench()
	client := &agentclient.Client{}
	w.agent.client = client
	signature := w.workspaceSignature()
	w.requestName.SetText("Local edit")
	w.agent.workspacePending = &agentWorkspaceResult{
		client: client, signature: signature, etag: "\"remote\"",
		value: project.Workspace{Version: 1, Draft: project.Draft{Name: "Remote"}},
	}
	w.finishAgentWorkspaceLoad()
	if w.draft().Name != "Local edit" || w.agent.workspaceETag != "" {
		t.Fatal("late Agent workspace overwrote local edits")
	}
}

func TestAgentWorkspaceSaveFinishesOutsideAgentPanel(t *testing.T) {
	w := newWorkbench()
	client := &agentclient.Client{}
	w.agent.client = client
	w.agent.open = false
	w.agent.workspacePending = &agentWorkspaceResult{client: client, saved: true, etag: "\"saved\""}
	w.finishAgentWorkspaceLoad()
	if w.agent.workspaceETag != "\"saved\"" || w.agent.status != "Agent workspace saved" {
		t.Fatal("save result was lost while the Agent panel was closed")
	}
}

func TestAgentWorkspaceDeleteClearsRemoteVersion(t *testing.T) {
	w := newWorkbench()
	client := &agentclient.Client{}
	w.agent.client = client
	w.agent.workspaceETag = "\"old\""
	w.requestName.SetText("Local draft")
	w.agent.workspacePending = &agentWorkspaceResult{client: client, deleted: true}
	w.finishAgentWorkspaceLoad()
	if w.agent.workspaceETag != "" || w.agent.status != "Agent workspace deleted" || w.draft().Name != "Local draft" {
		t.Fatal("delete result did not clear the remote version while retaining local work")
	}
}
