package main

import (
	"context"
	"fmt"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"
	"protovoy.dev/protovoy/internal/agentclient"
	"protovoy.dev/protovoy/internal/mock"
)

var (
	mockPanelID  = tree.ID(page, "mock-rules")
	mockEditorID = tree.ID(page, "mock-editor")
	mockCheckID  = tree.ID(page, "mock-check")
	mockExportID = tree.ID(page, "mock-export")
	mockBackID   = tree.ID(page, "mock-back")
	mockImportID = tree.ID(page, "mock-import")
	mockStartID  = tree.ID(page, "mock-start")
	mockStopID   = tree.ID(page, "mock-stop")
	mockStatusID = tree.ID(page, "mock-status")
)

type mockPanel struct {
	open           bool
	editor         requestEditor
	message        string
	picking        bool
	pending        *mockImportResult
	runtimePending *mockRuntimeResult
	busy           bool
	address        string
}

type mockRuntimeResult struct {
	client *agentclient.Client
	status agentclient.MockStatus
	err    error
}

func (w *workbench) manageMock(action string) {
	client := w.agent.client
	data := []byte(w.mock.editor.input.Editor.String())
	w.mock.busy = true
	go func() {
		var status agentclient.MockStatus
		var err error
		switch action {
		case "start":
			status, err = client.StartMock(context.Background(), data)
		case "stop":
			status, err = client.StopMock(context.Background())
		default:
			status, err = client.MockStatus(context.Background())
		}
		w.mu.Lock()
		w.mock.runtimePending = &mockRuntimeResult{client, status, err}
		w.mu.Unlock()
		w.invalidator.Invalidate()
	}()
}

func (w *workbench) finishMockRuntime() {
	w.mu.Lock()
	result := w.mock.runtimePending
	w.mock.runtimePending = nil
	w.mu.Unlock()
	if result == nil {
		return
	}
	w.mock.busy = false
	if result.client != w.agent.client {
		w.mock.address = ""
		return
	}
	if result.err != nil {
		w.mock.message = result.err.Error()
		return
	}
	w.mock.address = result.status.Address
	w.mock.message = "Mock stopped"
	if result.status.Running {
		w.mock.message = "Mock running at http://" + result.status.Address
	}
}

type mockImportResult struct {
	data []byte
	err  error
}

func (w *workbench) importMock() {
	w.mock.picking = true
	chooseLocalFile(app.OpenFileOptions{Title: "Import Mock rules", Filters: []app.FileFilter{{Name: "JSON", Pattern: "*.json"}}}, 8<<20, func(data []byte, err error) {
		if data != nil && err == nil {
			rules, parseErr := mock.Parse(data)
			err = parseErr
			if err == nil {
				data, err = mock.Encode(rules)
			}
		}
		w.mu.Lock()
		w.mock.pending = &mockImportResult{data, err}
		w.mu.Unlock()
		w.invalidator.Invalidate()
	})
}

func (w *workbench) finishMockImport() {
	w.mu.Lock()
	result := w.mock.pending
	w.mock.pending = nil
	w.mu.Unlock()
	if result == nil {
		return
	}
	w.mock.picking = false
	if result.err != nil {
		w.mock.message = result.err.Error()
	} else if result.data != nil {
		w.mock.editor = newRequestEditor()
		w.mock.editor.input.Editor = text.NewEditor(string(result.data))
		w.mock.message = "Mock rules imported"
	}
}

func (w *workbench) drawMock(f *flex.Frame, canvas *ui.Canvas, compact bool) {
	panel := &w.mock
	f.Text("Mock rules")
	if panel.picking {
		f.Text("Opening Mock rules…")
		return
	}
	panel.editor.draw(f, canvas, mockEditorID, "Scenario JSON", "JSON array of Mock scenarios").Height(280)
	workbenchRow(f, compact, 140).Children(func() {
		if f.Button(mockImportID, "Import Mock rules").Clicked() {
			w.importMock()
		}
		check := f.Button(mockCheckID, "Validate rules").Clicked()
		export := f.Button(mockExportID, "Export Mock rules").Clicked()
		if check || export {
			rules, err := mock.Parse([]byte(panel.editor.input.Editor.String()))
			if err != nil {
				panel.message = err.Error()
			} else {
				panel.message = fmt.Sprintf("%d Mock rules validated", len(rules))
				if export {
					data, err := mock.Encode(rules)
					if err == nil {
						var saved bool
						saved, err = saveProjectFile("mock-rules.json", data)
						if saved {
							panel.message = "Mock rules exported"
						}
					}
					if err != nil {
						panel.message = err.Error()
					}
				}
			}
		}
		if f.Button(mockBackID, "Back to workbench").Clicked() {
			panel.open = false
		}
	})
	f.Wrap(panel.message)
	if w.agent.client != nil && w.agent.client.SupportsMock() && !panel.busy {
		workbenchRow(f, compact, 140).Children(func() {
			if f.Button(mockStartID, "Start Mock via Agent").Clicked() {
				w.manageMock("start")
			}
			if f.Button(mockStopID, "Stop Mock via Agent").Clicked() {
				w.manageMock("stop")
			}
			if f.Button(mockStatusID, "Refresh Mock status").Clicked() {
				w.manageMock("status")
			}
		})
	}
	if panel.busy {
		f.Text("Updating Mock service…")
	}
}
