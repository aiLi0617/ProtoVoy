package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/vivaui/viva/flex"

	"protovoy.dev/protovoy/internal/agentclient"
	"protovoy.dev/protovoy/internal/agenthistory"
	"protovoy.dev/protovoy/internal/project"
)

var (
	historyPanelID         = tree.ID(page, "history-panel")
	historyRefreshID       = tree.ID(page, "history-refresh")
	historyScrollID        = tree.ID(page, "history-scroll")
	historyBodyID          = tree.ID(page, "history-body")
	historyRequestID       = tree.ID(page, "history-open-request")
	historySnapshotsID     = tree.ID(page, "history-snapshots")
	historySnapshotOpenID  = tree.ID(page, "history-open-snapshot")
	historySnapshotClearID = tree.ID(page, "history-clear-snapshots")
	historyRows            = tree.Sub(page, tree.Name("history-rows"))
)

type historyPanel struct {
	open     bool
	records  []agenthistory.Record
	selected string
	status   string
	bodyID   string
	body     agentclient.HistoryResponse
	pending  *historyResult // protected by workbench.mu
}

type historyResult struct {
	records []agenthistory.Record
	bodyID  string
	body    agentclient.HistoryResponse
	err     error
}

func (w *workbench) agentExecutionContext() context.Context {
	ctx := context.Background()
	state := &w.collection
	if !w.useAgent() || state.project == nil || state.selected == 0 || w.draft() != state.baseline {
		return ctx
	}
	request := &state.project.Requests[state.selected-1]
	if request.LocalID == "" {
		request.LocalID = agentclient.NewRequestID()
		state.revision++
	}
	ctx = agentclient.WithSavedRequestID(ctx, request.LocalID)
	if w.historySnapshotsEnabled {
		if id := w.captureHistoryRequest(*request); id != "" {
			ctx = agentclient.WithHistorySnapshotID(ctx, id)
		}
	}
	return ctx
}

func (w *workbench) captureHistoryRequest(request project.Request) string {
	data, err := project.CaptureHistoryRequest(request)
	if err != nil {
		return ""
	}
	total := len(data)
	for _, item := range w.historySnapshots {
		if bytes.Equal(item.Request, data) {
			return item.ID
		}
		total += len(item.Request)
	}
	for len(w.historySnapshots) >= project.MaxHistorySnapshots || total > project.MaxHistorySnapshotTotal {
		total -= len(w.historySnapshots[0].Request)
		w.historySnapshots[0] = project.HistorySnapshot{}
		w.historySnapshots = w.historySnapshots[1:]
	}
	id := agentclient.NewRequestID()
	w.historySnapshots = append(w.historySnapshots, project.HistorySnapshot{ID: id, Request: data})
	w.collection.revision++
	return id
}

func (w *workbench) historySnapshot(id string) *project.HistorySnapshot {
	for index := range w.historySnapshots {
		if w.historySnapshots[index].ID == id {
			return &w.historySnapshots[index]
		}
	}
	return nil
}

func (w *workbench) openHistorySnapshot(snapshot project.HistorySnapshot) {
	if !w.canLeaveDraft() {
		w.history.status = w.collection.message
		return
	}
	request, err := project.OpenHistoryRequest(snapshot.Request)
	if err != nil {
		w.history.status = err.Error()
		return
	}
	w.history.open = false
	w.loadRequest(0, request)
	w.collection.revision++
	w.collection.message = "Opened historical request as a new draft"
	w.invalidator.Invalidate()
}

func (w *workbench) historyRequestIndex(id string) int {
	if id == "" || w.collection.project == nil {
		return -1
	}
	found := -1
	for index, request := range w.collection.project.Requests {
		if request.LocalID == id {
			if found >= 0 {
				return -1
			}
			found = index
		}
	}
	return found
}

func (w *workbench) openHistoryRequest(index int) {
	if !w.canLeaveDraft() {
		w.history.status = w.collection.message
		return
	}
	w.history.open = false
	w.collection.page = index / collectionPageSize
	w.loadRequest(index+1, w.collection.project.Requests[index])
	w.collection.message = "Opened current saved request from history"
	w.invalidator.Invalidate()
}

func (w *workbench) loadHistory() {
	client := w.agent.client
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.history.status = "Loading Local Agent history…"
	go func() {
		defer cancel()
		records, err := client.History(ctx)
		w.mu.Lock()
		w.history.pending = &historyResult{records: records, err: err}
		w.mu.Unlock()
		w.invalidator.Invalidate()
	}()
}

func (w *workbench) loadHistoryBody(id string) {
	client := w.agent.client
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.history.status = "Loading captured response…"
	go func() {
		defer cancel()
		body, err := client.HistoryResponse(ctx, id)
		w.mu.Lock()
		w.history.pending = &historyResult{bodyID: id, body: body, err: err}
		w.mu.Unlock()
		w.invalidator.Invalidate()
	}()
}

func (w *workbench) finishHistory(load *historyResult) {
	w.cancel = nil
	if load.err != nil {
		w.history.status = load.err.Error()
		return
	}
	if load.bodyID != "" {
		w.history.bodyID, w.history.body = load.bodyID, load.body
		w.history.status = ""
		return
	}
	w.history.records = load.records
	w.history.status = ""
	if len(load.records) == 0 {
		w.history.status = "No Local Agent executions yet"
	}
}

func (w *workbench) drawHistory(f *flex.Frame) {
	panel := &w.history
	f.Text("Local Agent history")
	f.Row().Gap(8).Children(func() {
		if w.cancel != nil {
			return
		}
		if f.Button(historyRefreshID, "Refresh history").Clicked() {
			w.loadHistory()
		}
		label := "Enable request snapshots"
		if w.historySnapshotsEnabled {
			label = "Disable request snapshots"
		}
		if f.Button(historySnapshotsID, label).Clicked() {
			w.historySnapshotsEnabled = !w.historySnapshotsEnabled
		}
		if len(w.historySnapshots) != 0 && f.Button(historySnapshotClearID, "Clear request snapshots").Clicked() {
			w.historySnapshots = nil
			w.collection.revision++
		}
	})
	f.Wrap("Snapshots keep saved request templates in this workspace. Runtime credentials and resolved variables are excluded.").Muted()
	if panel.status != "" {
		f.Wrap(panel.status).Muted()
	}
	f.ScrollArea(historyScrollID).Grow(1).Children(func() {
		f.Column().Gap(6).Children(func() {
			for i, record := range panel.records {
				label := strings.ToUpper(record.ProtocolID) + " · " + record.State
				if record.Status != nil {
					label += fmt.Sprintf(" · %d", *record.Status)
				}
				label += " · " + record.FinishedAt.Local().Format("Jan 2 15:04:05")
				if f.Button(historyRows.At(i), label).Clicked() {
					panel.selected = record.ID
					panel.bodyID, panel.body = "", agentclient.HistoryResponse{}
				}
				if panel.selected == record.ID {
					f.Wrap(fmt.Sprintf("%d ms · %d response bytes", record.DurationMS, record.BytesReceived))
					f.Wrap("Execution " + record.ID).Muted()
					if snapshot := w.historySnapshot(record.SnapshotID); snapshot != nil && w.cancel == nil &&
						f.Button(historySnapshotOpenID, "Open historical request").Clicked() {
						w.openHistorySnapshot(*snapshot)
					}
					if index := w.historyRequestIndex(record.RequestID); index >= 0 {
						if w.cancel == nil && f.Button(historyRequestID, "Open saved request").Clicked() {
							w.openHistoryRequest(index)
						}
					}
					if record.ResponseBodyBytes != nil {
						w.drawHistoryBody(f, record)
					}
				}
			}
		})
	})
}

func (w *workbench) drawHistoryBody(f *flex.Frame, record agenthistory.Record) {
	f.Wrap(fmt.Sprintf("%d captured bytes", *record.ResponseBodyBytes)).Muted()
	if w.history.bodyID != record.ID {
		if w.cancel == nil && f.Button(historyBodyID, "View captured response").Clicked() {
			w.loadHistoryBody(record.ID)
		}
		return
	}
	body := w.history.body
	title, data := "Captured response (Base64)", base64.StdEncoding.EncodeToString(body.Data)
	if body.Text {
		title, data = "Captured response (text)", strings.ToValidUTF8(string(body.Data), "\uFFFD")
	}
	if body.Truncated {
		title += " · preview limited to 8 KiB"
	} else if record.ResponseBodyTruncated {
		title += " · capture truncated"
	}
	f.Wrap(title)
	f.Wrap(data)
}
