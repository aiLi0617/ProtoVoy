package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

var (
	textBodyID             = tree.ID(page, "http-text-body")
	base64BodyID           = tree.ID(page, "http-base64-body")
	multipartModeID        = tree.ID(page, "http-multipart-mode")
	multipartBodyID        = tree.ID(page, "http-multipart-body")
	httpBodySourceID       = tree.ID(page, "http-body-source")
	httpBodySourceScrollID = tree.ID(page, "http-body-source-scroll")
	httpBodySourceEditorID = tree.ID(page, "http-body-source-editor")
	httpBodySourceApplyID  = tree.ID(page, "http-body-source-apply")
	httpBodySourceCancelID = tree.ID(page, "http-body-source-cancel")
	messagePackBodyID      = tree.ID(page, "http-messagepack-body")
	httpTimeoutID          = tree.ID(page, "http-timeout")
	responseBodyContainsID = tree.ID(page, "response-body-contains")
	expectedHTTPStatusID   = tree.ID(page, "expected-http-status")
)

func (w *workbench) currentHTTPBodySource() string {
	if w.httpBodySource == "" || w.httpSourceMode != w.httpBodyMode {
		return ""
	}
	body := w.requestBody.input.Editor.String()
	if w.httpBodyMode == "multipart" {
		body = w.multipartBody.input.Editor.String()
	}
	if body != w.httpSourceBody {
		return ""
	}
	return w.httpBodySource
}

func prepareHTTPBody(draft project.Draft, payload execution.HTTPPayload) (execution.HTTPPayload, error) {
	timeout, err := parseRequestTimeout(draft.HTTPTimeoutMS)
	if err != nil {
		return execution.HTTPPayload{}, err
	}
	payload.TimeoutMS = timeout
	switch draft.HTTPBodyMode {
	case "base64":
		payload.BodyEncoding = "base64"
	case "multipart":
		parts, err := project.ParseHTTPMultipart(draft.MultipartJSON)
		if err != nil {
			return execution.HTTPPayload{}, err
		}
		payload.Multipart = parts
		payload.Body = ""
	}
	return payload, nil
}

func parseRequestTimeout(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	timeout, err := strconv.Atoi(value)
	if err != nil || timeout < 1 || !execution.ValidRequestTimeout(timeout) {
		return 0, errors.New("enter a timeout from 1 to 300,000 ms")
	}
	return timeout, nil
}

func (w *workbench) applyHTTPSource(kind, source string) error {
	current := w.currentHTTPBodySource()
	currentKind := execution.HTTPSourceKind(w.httpBodyMode, current)
	if source != "" && source == current && kind == currentKind {
		w.showHTTPBodySource = false
		return nil
	}
	body, err := execution.EncodeHTTPSource(kind, source)
	if err != nil {
		return err
	}
	if kind == "messagepack" && currentKind != kind {
		if err := w.configureMessagePackBody(); err != nil {
			return err
		}
	}
	w.requestBody.input.Editor = text.NewEditor(body)
	w.httpBodySource, w.httpSourceBody, w.httpSourceMode = source, body, w.httpBodyMode
	w.showHTTPBodySource = false
	return nil
}

func (w *workbench) configureMessagePackBody() error {
	raw := w.headers.input.Editor.String()
	headers, err := execution.ParseHeaders(raw)
	if err != nil {
		return err
	}
	hasContentType := false
	for _, header := range headers {
		hasContentType = hasContentType || strings.EqualFold(header[0], "Content-Type")
	}
	if !hasContentType {
		if raw != "" && !strings.HasSuffix(raw, "\n") {
			raw += "\n"
		}
		w.headers.input.Editor = text.NewEditor(raw + "Content-Type: application/msgpack")
	}
	if strings.TrimSpace(w.method.Text()) == "" {
		w.method.SetText(http.MethodPost)
	}
	w.httpBodyMode = "base64"
	return nil
}

func (w *workbench) openHTTPSource(kind, source string) {
	w.showHTTPBodySource = true
	w.httpSourceKind, w.httpSourceNotice = kind, ""
	w.httpSourceEditor.input.Editor = text.NewEditor(source)
}

func (w *workbench) drawHTTPBody(f *flex.Frame, canvas *ui.Canvas, compact bool) {
	source := w.currentHTTPBodySource()
	if w.showHTTPBodySource {
		w.drawHTTPSource(f, canvas, w.httpSourceKind, source, compact)
		return
	}
	kind := execution.HTTPSourceKind(w.httpBodyMode, source)
	if w.httpBodyMode == "multipart" {
		kind = ""
	}
	workbenchRow(f, compact, 168).Children(func() {
		f.Text("Body format")
		for _, option := range []struct {
			id          ui.ID
			mode, label string
		}{
			{textBodyID, "", "Text"},
			{base64BodyID, "base64", "Base64"},
			{multipartModeID, "multipart", "Multipart"},
		} {
			label := option.label
			if option.mode == w.httpBodyMode {
				label += " (selected)"
			}
			if f.Button(option.id, label).Clicked() {
				w.httpBodyMode = option.mode
				w.showHTTPBodySource = false
			}
		}
		if source != "" && w.cancel == nil {
			label := "View original source"
			if kind != "" {
				label = "Edit body source"
			}
			if f.Button(httpBodySourceID, label).Clicked() {
				w.openHTTPSource(kind, source)
			}
		}
		if kind != "messagepack" && w.cancel == nil && f.Button(messagePackBodyID, "MessagePack").Clicked() {
			w.openHTTPSource("messagepack", "")
		}
	})
	f.Field(&w.httpTimeout, httpTimeoutID, "HTTP timeout in milliseconds", "Timeout (ms), default 30000")
	w.drawHTTPAssertions(f)
	switch w.httpBodyMode {
	case "multipart":
		f.Wrap("Remove any Content-Type header. The boundary is added when sending.").Muted()
		w.multipartBody.draw(f, canvas, multipartBodyID, "HTTP multipart parts JSON",
			`[{"name":"field","value":"hello"}]`).Height(100)
	case "base64":
		w.requestBody.draw(f, canvas, requestBodyID, "Base64 request body", "Enter Base64 bytes").Height(84)
	default:
		w.requestBody.draw(f, canvas, requestBodyID, "Request body", "Enter a request body").Height(84)
	}
}

func (w *workbench) drawHTTPAssertions(f *flex.Frame) {
	f.Field(&w.expectedHTTPStatus, expectedHTTPStatusID, "Expected HTTP status for collection runs", "Expected HTTP status (collection runs)")
	f.Field(&w.responseBodyContains, responseBodyContainsID, "Expected response text for collection runs", "Response body contains (collection runs)")
}

func (w *workbench) drawHTTPSource(f *flex.Frame, canvas *ui.Canvas, kind, source string, compact bool) {
	if kind == "" {
		f.Wrap("Imported source · read only").Muted()
		f.ScrollArea(httpBodySourceScrollID).Height(100).Children(func() { f.Wrap(source) })
		if f.Button(httpBodySourceCancelID, "Back to request body").Clicked() {
			w.showHTTPBodySource = false
		}
		return
	}
	w.httpSourceEditor.draw(f, canvas, httpBodySourceEditorID, "HTTP body source JSON", "Enter source JSON").Height(140)
	workbenchRow(f, compact, 168).Children(func() {
		if f.Button(httpBodySourceApplyID, "Apply source changes").Clicked() {
			if err := w.applyHTTPSource(kind, w.httpSourceEditor.input.Editor.String()); err != nil {
				w.httpSourceNotice = err.Error()
			}
		}
		if f.Button(httpBodySourceCancelID, "Discard source edits").Clicked() {
			w.showHTTPBodySource = false
		}
	})
	f.Wrap(w.httpSourceNotice)
}
