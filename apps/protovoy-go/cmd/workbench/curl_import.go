package main

import (
	"strconv"

	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
	"protovoy.dev/protovoy/internal/project"
)

var (
	curlImportID       = tree.ID(page, "import-curl")
	curlImportEditorID = tree.ID(page, "import-curl-editor")
	curlImportApplyID  = tree.ID(page, "import-curl-apply")
	curlImportCancelID = tree.ID(page, "import-curl-cancel")
)

type curlImportPanel struct {
	open    bool
	editor  requestEditor
	message string
}

func (w *workbench) drawCurlImport(f *flex.Frame, canvas *ui.Canvas, compact bool) {
	panel := &w.curlImport
	panel.editor.draw(f, canvas, curlImportEditorID, "cURL command", "Paste a cURL command").Height(160)
	workbenchRow(f, compact, 140).Children(func() {
		if f.Button(curlImportApplyID, "Load request draft").Clicked() {
			request, err := project.ParseCurl(panel.editor.input.Editor.String())
			if err != nil {
				panel.message = err.Error()
			} else {
				w.loadCurlDraft(request)
				w.curlImport = curlImportPanel{}
			}
		}
		if f.Button(curlImportCancelID, "Cancel import").Clicked() {
			w.curlImport = curlImportPanel{}
		}
	})
	if panel.message != "" {
		f.Wrap(panel.message)
	}
}

func (w *workbench) loadCurlDraft(request project.Request) {
	w.loadRequest(0, project.Request{})
	draft := w.collection.baseline
	draft.Name, draft.Target, draft.Method = request.Name, request.URL, request.Method
	draft.Headers, draft.Body = request.HeaderText(), request.Body
	draft.FollowRedirects = request.FollowRedirects
	if request.HTTPTimeoutMS != 0 {
		draft.HTTPTimeoutMS = strconv.Itoa(request.HTTPTimeoutMS)
	}
	w.applyDraft(draft)
	w.collection.message = "cURL loaded into a new request draft"
}
