package main

import (
	"strings"
	"testing"

	"protovoy.dev/protovoy/internal/project"
)

func TestCredentialPanelStaysOutsideWorkspaceSnapshot(t *testing.T) {
	w := newWorkbench()
	w.secrets.reference.SetText("private-reference-fixture")
	w.secrets.value.SetText("private-credential-fixture")
	data, err := project.EncodeWorkspace(w.currentWorkspace())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "private-reference-fixture") || strings.Contains(string(data), "private-credential-fixture") {
		t.Fatal("credential management fields entered workspace persistence")
	}
}

func TestHTTPBindingReferenceClearsWhenLoadingAnotherRequest(t *testing.T) {
	w := newWorkbench()
	w.requestSecret.SetText("fixture")
	w.requestSecretHeader.SetText("Authorization")
	w.loadRequest(0, project.Request{Method: "GET", URL: "https://example.test/next"})
	if w.requestSecret.Text() != "" || w.requestSecretHeader.Text() != "" {
		t.Fatal("HTTP credential binding carried over to another request")
	}
}

func TestWorkspaceRestoresHTTPBindingReference(t *testing.T) {
	w := newWorkbench()
	w.requestSecret.SetText("fixture")
	w.requestSecretHeader.SetText("Authorization")
	data, err := project.EncodeWorkspace(w.currentWorkspace())
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := project.DecodeWorkspace(data)
	if err != nil {
		t.Fatal(err)
	}
	restored := newWorkbench()
	restored.applyWorkspace(workspace)
	if restored.requestSecret.Text() != "fixture" || restored.requestSecretHeader.Text() != "Authorization" {
		t.Fatal("workspace did not restore the HTTP binding reference")
	}
}

func TestSavedRequestRestoresHTTPBinding(t *testing.T) {
	w := newWorkbench()
	w.target.SetText("https://example.test/")
	w.requestSecret.SetText("fixture")
	w.requestSecretHeader.SetText("Authorization")
	w.saveDraft()
	if w.collection.project == nil || len(w.collection.project.Requests) != 1 {
		t.Fatalf("request was not saved: %s", w.collection.message)
	}
	request := w.collection.project.Requests[0]
	w.loadRequest(0, project.Request{})
	w.loadRequest(1, request)
	if w.requestSecret.Text() != "fixture" || w.requestSecretHeader.Text() != "Authorization" {
		t.Fatal("saved request lost its credential binding")
	}
}
