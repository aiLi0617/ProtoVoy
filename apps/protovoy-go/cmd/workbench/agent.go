package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vivaui/viva/flex"

	"protovoy.dev/protovoy/internal/agentclient"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

var (
	agentPanelID           = tree.ID(page, "local-agent")
	agentBackID            = tree.ID(page, "local-agent-back")
	agentAddressID         = tree.ID(page, "agent-address")
	agentTokenID           = tree.ID(page, "agent-token")
	agentPairID            = tree.ID(page, "agent-pair")
	agentForgetID          = tree.ID(page, "agent-forget")
	agentWorkspaceLoadID   = tree.ID(page, "agent-workspace-load")
	agentWorkspaceSaveID   = tree.ID(page, "agent-workspace-save")
	agentWorkspaceDeleteID = tree.ID(page, "agent-workspace-delete")
)

type agentPanel struct {
	open             bool
	address, token   flex.Field
	client           *agentclient.Client
	status           string
	pending          *agentPairResult      // protected by workbench.mu
	workspacePending *agentWorkspaceResult // protected by workbench.mu
	workspaceETag    string
}

type agentWorkspaceResult struct {
	deleted   bool
	saved     bool
	client    *agentclient.Client
	value     project.Workspace
	etag      string
	signature workspaceSignature
	err       error
}

type agentPairResult struct {
	client *agentclient.Client
	err    error
}

func (w *workbench) drawAgent(f *flex.Frame) {
	panel := &w.agent
	panel.token.Secure = true
	f.Text("Local Agent")
	f.Field(&panel.address, agentAddressID, "Local Agent address", agentclient.DefaultAddress)
	f.Field(&panel.token, agentTokenID, "Local Agent pairing token", "Pairing token")
	f.Wrap("Pairing enables HTTP, SSE, WebSocket, TCP, UDP, Redis and gRPC execution on this machine. The token and session stay in memory.").Muted()
	f.Row().Gap(8).Children(func() {
		if w.cancel != nil {
			if f.Button(cancelID, "Cancel pairing").Clicked() {
				w.cancel()
			}
			return
		}
		if f.Button(agentPairID, "Pair with Local Agent").Clicked() {
			w.pairAgent()
		}
		if panel.client != nil && f.Button(agentForgetID, "Use default execution").Clicked() {
			panel.client = nil
			panel.workspaceETag = ""
			w.history = historyPanel{}
			panel.token.SetText("")
			panel.status = "Using default execution"
		}
		if panel.client != nil && panel.client.SupportsWorkspace() && f.Button(agentWorkspaceLoadID, "Load Agent workspace").Clicked() && w.canLeaveDraft() {
			w.loadAgentWorkspace()
		}
		if panel.client != nil && panel.client.SupportsWorkspace() {
			label := "Save Agent workspace"
			if panel.workspaceETag == "" {
				label = "Create Agent workspace"
			}
			if f.Button(agentWorkspaceSaveID, label).Clicked() {
				w.saveAgentWorkspace()
			}
		}
		if panel.client != nil && panel.workspaceETag != "" && f.Button(agentWorkspaceDeleteID, "Delete Agent workspace").Clicked() {
			w.deleteAgentWorkspace()
		}
	})
	f.Wrap(panel.status)
	w.drawSecrets(f)
	w.drawPlugins(f)
}

func (w *workbench) loadAgentWorkspace() {
	client := w.agent.client
	signature := w.workspaceSignature()
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.agent.status = "Loading Agent workspace…"
	go func() {
		defer cancel()
		value, etag, err := client.Workspace(ctx)
		w.mu.Lock()
		w.agent.workspacePending = &agentWorkspaceResult{client: client, value: value, etag: etag, signature: signature, err: err}
		w.mu.Unlock()
		w.invalidator.Invalidate()
	}()
}

func (w *workbench) finishAgentWorkspaceLoad() {
	w.mu.Lock()
	result := w.agent.workspacePending
	w.agent.workspacePending = nil
	w.mu.Unlock()
	if result == nil {
		return
	}
	w.cancel = nil
	if result.client != w.agent.client {
		return
	}
	if result.err != nil {
		w.agent.status = result.err.Error()
		return
	}
	if result.deleted {
		w.agent.workspaceETag = ""
		w.agent.status = "Agent workspace deleted"
		return
	}
	if result.saved {
		w.agent.workspaceETag = result.etag
		w.agent.status = "Agent workspace saved"
		return
	}
	if result.signature != w.workspaceSignature() {
		w.agent.status = "Workspace edited while loading. Load again to apply Agent workspace."
		return
	}
	w.applyWorkspace(result.value)
	w.agent.workspaceETag = result.etag
	w.agent.status = "Agent workspace loaded"
}

func (w *workbench) deleteAgentWorkspace() {
	client, etag := w.agent.client, w.agent.workspaceETag
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.agent.status = "Deleting Agent workspace…"
	go func() {
		defer cancel()
		err := client.DeleteWorkspace(ctx, etag)
		w.mu.Lock()
		w.agent.workspacePending = &agentWorkspaceResult{client: client, deleted: true, err: err}
		w.mu.Unlock()
		w.invalidator.Invalidate()
	}()
}

func (w *workbench) saveAgentWorkspace() {
	data, err := project.EncodeWorkspace(w.currentWorkspace())
	if err != nil {
		w.agent.status = err.Error()
		return
	}
	value, err := project.DecodeWorkspace(data)
	if err != nil {
		w.agent.status = err.Error()
		return
	}
	client, etag := w.agent.client, w.agent.workspaceETag
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.agent.status = "Saving Agent workspace…"
	go func() {
		defer cancel()
		var err error
		if etag == "" {
			err = client.CreateWorkspace(ctx, value)
		} else {
			err = client.SaveWorkspace(ctx, value, etag)
		}
		version := fmt.Sprintf("\"%x\"", sha256.Sum256(data))
		w.mu.Lock()
		w.agent.workspacePending = &agentWorkspaceResult{client: client, saved: true, etag: version, err: err}
		w.mu.Unlock()
		w.invalidator.Invalidate()
	}()
}

func (w *workbench) pairAgent() {
	address, token := strings.TrimSpace(w.agent.address.Text()), w.agent.token.Text()
	w.agent.token.SetText("")
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.agent.status = "Pairing with Local Agent…"
	go func() {
		defer cancel()
		client, err := agentclient.Pair(ctx, address, token, doAgentRequest)
		w.mu.Lock()
		w.agent.pending = &agentPairResult{client, err}
		w.mu.Unlock()
		w.invalidator.Invalidate()
	}()
}

func (w *workbench) finishAgentPair(pair *agentPairResult) {
	w.cancel = nil
	switch {
	case errors.Is(pair.err, context.Canceled):
		w.agent.status = "Pairing cancelled"
	case pair.err != nil:
		w.agent.status = pair.err.Error()
	default:
		w.agent.client = pair.client
		w.agent.workspaceETag = ""
		w.history = historyPanel{}
		w.agent.status = "Paired · Local Agent ready"
	}
}

func (w *workbench) useAgent() bool {
	return w.agent.client != nil
}

func readAgentResponse(ctx context.Context, client *agentclient.Client, target string, payload execution.HTTPPayload, graphql bool, publish func(result)) result {
	return readAgentHTTPResult(ctx, graphql, publish, func(update func(agentclient.Response)) (agentclient.Response, error) {
		return client.RunHTTP(ctx, target, payload, update)
	})
}

func readAgentHTTPResult(ctx context.Context, graphql bool, publish func(result), run func(func(agentclient.Response)) (agentclient.Response, error)) result {
	started := time.Now()
	var headersTime time.Duration
	format := func(response agentclient.Response) result {
		value := result{status: "Waiting for Local Agent…", body: response.Preview, headers: response.Headers, previewBytes: response.PreviewBytes}
		if response.Status != 0 {
			if headersTime == 0 {
				headersTime = time.Since(started)
			}
			value.headersTime = headersTime
			value.status = fmt.Sprintf("HTTP %d · Local Agent", response.Status)
		}
		if response.Truncated {
			value.status += " · preview limited to 8 KiB"
		}
		return value
	}
	response, err := run(func(response agentclient.Response) {
		publish(format(response))
	})
	value := format(response)
	value.done = true
	if value.headersTime > 0 {
		value.previewTime = time.Since(started)
	}
	if err == nil && ctx.Err() == nil && !response.Truncated {
		value.formattedJSON = formatResponseJSON(response.Preview)
	}
	if err == nil && ctx.Err() == nil {
		value.rawPreview = response.RawPreview
	}
	switch {
	case ctx.Err() == context.Canceled || errors.Is(err, context.Canceled):
		if response.Status == 0 {
			value.status = "Cancelled"
		} else {
			value.status += " · cancelled"
		}
	case err != nil:
		message := err.Error()
		if errors.Is(err, context.DeadlineExceeded) {
			message = "Request timed out"
		}
		if response.Status == 0 {
			value.status = message
		} else {
			value.status += " · " + message
		}
	case graphql && !response.Truncated:
		if count := execution.GraphQLErrorCount([]byte(response.Preview)); count != 0 {
			value.status += fmt.Sprintf(" · GraphQL errors: %d", count)
		}
	}
	return value
}
