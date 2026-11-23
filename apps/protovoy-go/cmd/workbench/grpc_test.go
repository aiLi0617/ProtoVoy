package main

import (
	"bytes"
	"encoding/base64"
	"testing"

	"protovoy.dev/protovoy/internal/project"
)

func TestGRPCJSONKeepsDescriptorAcrossProjectAndWorkspace(t *testing.T) {
	w := newWorkbench()
	w.loadRequest(0, project.Request{GRPC: &project.GRPCSettings{Mode: "unary"}})
	w.target.SetText("http://localhost:50051")
	w.grpc.service.SetText("demo.Echo")
	w.grpc.method.SetText("Watch")
	w.grpc.useJSON = true
	w.requestBody.input.Editor.Insert(`{"value":1}`)
	descriptor := []byte{10, 3, 10, 1, 'x'}
	w.finishGRPCDescriptorImport(&descriptorImport{data: descriptor})
	w.saveDraft()
	if w.collection.selected != 1 {
		t.Fatalf("could not save JSON request: %s", w.collection.message)
	}
	portable, err := project.Encode(*w.collection.project)
	if err != nil {
		t.Fatal(err)
	}
	value, err := project.Read(bytes.NewReader(portable))
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := project.EncodeWorkspace(project.Workspace{Draft: w.draft(), Project: &value, Selected: 1})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := project.DecodeWorkspace(workspace)
	if err != nil {
		t.Fatal(err)
	}
	restored := newWorkbench()
	restored.loadRequest(saved.Selected, saved.Project.Requests[0])
	if restored.draft() != saved.Draft || !restored.grpc.useJSON ||
		restored.grpc.descriptor != base64.StdEncoding.EncodeToString(descriptor) {
		t.Fatal("gRPC JSON format or descriptor was lost")
	}
}

func TestGRPCJSONCanSaveWithServerReflection(t *testing.T) {
	w := newWorkbench()
	w.loadRequest(0, project.Request{GRPC: &project.GRPCSettings{Mode: "unary"}})
	w.target.SetText("http://localhost:50051")
	w.grpc.service.SetText("demo.Echo")
	w.grpc.method.SetText("Watch")
	w.grpc.useJSON = true
	w.requestBody.input.Editor.Insert(`{"value":1}`)
	w.saveDraft()
	if w.collection.selected != 1 || !w.collection.project.Requests[0].GRPC.UseJSON {
		t.Fatalf("could not save request using reflection: %s", w.collection.message)
	}
}

func TestGRPCSaveAndRestoreCollectionDraft(t *testing.T) {
	w := newWorkbench()
	w.loadRequest(0, project.Request{GRPC: &project.GRPCSettings{Mode: "unary"}})
	w.requestName.SetText("Watch updates")
	w.target.SetText("http://localhost:50051")
	w.grpc.service.SetText("demo.Echo")
	w.grpc.method.SetText("Watch")
	w.grpc.mode.SetText("server_streaming")
	w.headers.input.Editor.Insert("x-fixture: example")
	w.requestBody.input.Editor.Insert("CAE=")
	w.saveDraft()
	if w.collection.selected != 1 || w.collection.project.Requests[0].GRPC == nil {
		t.Fatal("could not save gRPC request")
	}
	data, err := project.EncodeWorkspace(project.Workspace{Draft: w.draft(), Project: w.collection.project, Selected: w.collection.selected})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := project.DecodeWorkspace(data)
	if err != nil {
		t.Fatal(err)
	}
	restored := newWorkbench()
	restored.loadRequest(saved.Selected, saved.Project.Requests[0])
	if !restored.grpc.open || restored.draft() != saved.Draft || saved.Draft.Body != "CAE=" ||
		saved.Draft.Headers != "x-fixture: example" || saved.Draft.GRPC.Mode != "server_streaming" {
		t.Fatal("gRPC draft did not round-trip through the saved collection")
	}
}
