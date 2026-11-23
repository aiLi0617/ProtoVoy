package main

import (
	"crypto/sha256"
	"time"

	"protovoy.dev/protovoy/internal/project"
)

type workspaceSignature struct {
	draft                   project.Draft
	revision                uint64
	varRevision             uint64
	selected, page          int
	modified                bool
	historySnapshotsEnabled bool
}

type workspaceAutosave struct {
	observed   workspaceSignature
	saved      workspaceSignature
	digest     [sha256.Size]byte
	timer      *time.Timer
	due        time.Time
	notice     string
	saveNotice bool
}

func (w *workbench) workspaceSignature() workspaceSignature {
	state := &w.collection
	return workspaceSignature{w.draft(), state.revision, w.variables.revision, state.selected, state.page, state.modified, w.historySnapshotsEnabled}
}

func (w *workbench) restoreWorkspace() {
	data, err := readWorkspace()
	switch {
	case err != nil:
		w.autosave.notice = "Could not read local workspace storage"
	case len(data) > 0:
		value, err := project.DecodeWorkspace(data)
		if err != nil {
			w.autosave.notice = err.Error()
		} else {
			w.applyWorkspace(value)
		}
	}
	w.autosave.digest = sha256.Sum256(data)
	w.autosave.saved = w.workspaceSignature()
	w.autosave.observed = w.autosave.saved
}

func (w *workbench) trackWorkspace() {
	autosave := &w.autosave
	signature := w.workspaceSignature()
	if signature != autosave.observed {
		const delay = 350 * time.Millisecond
		autosave.observed = signature
		autosave.due = time.Now().Add(delay)
		if autosave.timer == nil {
			autosave.timer = time.AfterFunc(delay, w.invalidator.Invalidate)
		} else {
			autosave.timer.Reset(delay)
		}
	}
	if !autosave.due.IsZero() && !time.Now().Before(autosave.due) {
		autosave.due = time.Time{}
		w.flushWorkspace()
	}
}

func (w *workbench) flushWorkspace() {
	autosave := &w.autosave
	signature := w.workspaceSignature()
	if signature == autosave.saved {
		// Edits were reverted after a failed save. The stored state is current
		// again, so the notice from that attempt can be cleared.
		if autosave.saveNotice {
			w.setAutosaveNotice("")
		}
		return
	}
	workspace := w.currentWorkspace()
	data, err := project.EncodeWorkspace(workspace)
	if err != nil {
		w.setAutosaveNotice(err.Error())
		return
	}
	current, err := readWorkspace()
	switch {
	case err != nil:
		w.setAutosaveNotice("Could not read local workspace storage")
	case sha256.Sum256(current) != autosave.digest:
		w.setAutosaveNotice("Workspace changed in another window. Export this project to keep your changes.")
	case writeWorkspace(data) != nil:
		w.setAutosaveNotice("Could not autosave the workspace")
	default:
		autosave.digest = sha256.Sum256(data)
		autosave.saved = signature
		w.setAutosaveNotice("")
	}
}

func (w *workbench) setAutosaveNotice(message string) {
	w.autosave.saveNotice = message != ""
	if w.autosave.notice != message {
		w.autosave.notice = message
		w.invalidator.Invalidate()
	}
}

func (w *workbench) applyWorkspace(value project.Workspace) {
	w.loadRequest(0, project.Request{})
	state := &w.collection
	state.page, state.replace, state.message = 0, false, ""
	state.revision++
	w.variables.revision++
	state.project, state.modified = value.Project, value.Modified
	w.historySnapshotsEnabled, w.historySnapshots = value.HistorySnapshotsEnabled, value.HistorySnapshots
	w.variables.scopes = value.Variables
	if value.Project != nil {
		total := len(value.Project.Requests) + len(value.Project.EmptyFolders)
		state.page = min(value.Page, max(total-1, 0)/collectionPageSize)
	}
	if value.Selected != 0 {
		w.loadRequest(value.Selected, value.Project.Requests[value.Selected-1])
	} else if value.Draft.Redis.Protocol != 0 {
		w.loadRequest(0, project.Request{Redis: &project.RedisSettings{Protocol: 2}})
	} else if value.Draft.GRPC.Mode != "" {
		w.loadRequest(0, project.Request{GRPC: &project.GRPCSettings{Mode: "unary"}})
	} else if value.Draft.MQTT.Mode != "" {
		w.loadRequest(0, defaultMQTTRequest())
	} else if value.Draft.AMQP.Mode != "" {
		w.loadRequest(0, defaultAMQPRequest())
	} else if value.Draft.SQL {
		w.loadRequest(0, defaultSQLRequest())
	} else if value.Draft.Kafka.Mode != "" {
		w.loadRequest(0, defaultKafkaRequest())
	} else if value.Draft.RPC.Valid() {
		w.loadRequest(0, project.Request{RPC: &project.RPCSettings{Protocol: value.Draft.RPC.Protocol}})
	}
	w.applyDraft(value.Draft)
	if value.Selected == 0 {
		state.variables = value.DraftVariables
	}
}

func (w *workbench) currentWorkspace() project.Workspace {
	signature := w.workspaceSignature()
	workspace := project.Workspace{
		Project: w.collection.project, Draft: signature.draft,
		Selected: signature.selected, Page: signature.page, Modified: signature.modified,
		Variables:               w.variables.scopes,
		HistorySnapshotsEnabled: w.historySnapshotsEnabled, HistorySnapshots: w.historySnapshots,
	}
	if signature.selected == 0 {
		workspace.DraftVariables = w.collection.variables
	}
	return workspace
}
