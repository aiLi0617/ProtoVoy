package main

import (
	"context"

	"github.com/vivaui/viva/flex"
	"protovoy.dev/protovoy/internal/agentclient"
)

var (
	requestSecretID       = tree.ID(page, "request-http-secret")
	requestSecretHeaderID = tree.ID(page, "request-http-secret-header")
	secretReferenceID     = tree.ID(page, "secret-reference")
	secretValueID         = tree.ID(page, "secret-value")
	secretSaveID          = tree.ID(page, "secret-save")
	secretCheckID         = tree.ID(page, "secret-check")
	secretDeleteID        = tree.ID(page, "secret-delete")
	httpSecretTargetID    = tree.ID(page, "http-secret-target")
	httpSecretHeaderID    = tree.ID(page, "http-secret-header")
	httpSecretSaveID      = tree.ID(page, "http-secret-save")
	httpSecretDeleteID    = tree.ID(page, "http-secret-delete")
)

type secretPanel struct {
	reference, value flex.Field
	target, header   flex.Field
	busy             bool
	message          string
	pending          *secretResult
}

type secretResult struct {
	client  *agentclient.Client
	message string
}

func (w *workbench) drawSecrets(f *flex.Frame) {
	if w.agent.client == nil || !w.agent.client.SupportsSecrets() {
		return
	}
	panel := &w.secrets
	panel.value.Secure = true
	f.Text("System credentials")
	f.Field(&panel.reference, secretReferenceID, "Secret reference", "Reference name")
	f.Field(&panel.value, secretValueID, "Secret value", "Credential value")
	if !panel.busy {
		f.Row().Gap(8).Children(func() {
			if f.Button(secretSaveID, "Save secret").Clicked() {
				w.manageSecret("save")
			}
			if f.Button(secretCheckID, "Check secret").Clicked() {
				w.manageSecret("check")
			}
			if f.Button(secretDeleteID, "Delete secret").Clicked() {
				w.manageSecret("delete")
			}
		})
	}
	f.Text("HTTP credential binding")
	f.Field(&panel.target, httpSecretTargetID, "Bound URL", "Exact request URL")
	f.Field(&panel.header, httpSecretHeaderID, "Bound header", "Authorization")
	if !panel.busy {
		f.Row().Gap(8).Children(func() {
			if f.Button(httpSecretSaveID, "Bind HTTP credential").Clicked() {
				w.manageSecret("bind-http")
			}
			if f.Button(httpSecretDeleteID, "Delete HTTP binding").Clicked() {
				w.manageSecret("delete-http")
			}
		})
	}
	f.Wrap(panel.message)
}

func (w *workbench) manageSecret(action string) {
	client := w.agent.client
	reference, value := w.secrets.reference.Text(), w.secrets.value.Text()
	target, header := w.secrets.target.Text(), w.secrets.header.Text()
	w.secrets.value.SetText("")
	w.secrets.busy = true
	go func() {
		var err error
		message := ""
		switch action {
		case "bind-http":
			err = client.PutHTTPSecret(context.Background(), reference, target, header, value)
			message = "HTTP credential bound"
		case "delete-http":
			err = client.DeleteHTTPSecret(context.Background(), reference)
			message = "HTTP binding deleted"
		case "save":
			err = client.PutSecret(context.Background(), reference, value)
			message = "Secret saved"
		case "delete":
			err = client.DeleteSecret(context.Background(), reference)
			message = "Secret deleted"
		default:
			var exists bool
			exists, err = client.SecretExists(context.Background(), reference)
			message = "Secret not found"
			if exists {
				message = "Secret exists"
			}
		}
		if err != nil {
			message = err.Error()
		}
		w.mu.Lock()
		w.secrets.pending = &secretResult{client, message}
		w.mu.Unlock()
		w.invalidator.Invalidate()
	}()
}

func (w *workbench) finishSecret() {
	w.mu.Lock()
	result := w.secrets.pending
	w.secrets.pending = nil
	w.mu.Unlock()
	if result == nil {
		return
	}
	w.secrets.busy = false
	if result.client == w.agent.client {
		w.secrets.message = result.message
	}
}
