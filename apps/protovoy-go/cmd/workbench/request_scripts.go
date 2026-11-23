package main

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

var (
	requestScriptsID    = tree.ID(page, "request-scripts")
	preScriptsEditorID  = tree.ID(page, "pre-scripts-editor")
	postScriptsEditorID = tree.ID(page, "post-scripts-editor")
)

func formatRequestScripts(scripts []string) string {
	if len(scripts) == 0 {
		return ""
	}
	data, _ := json.MarshalIndent(scripts, "", "  ")
	return string(data)
}

func parseRequestScripts(source string) ([]string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil, nil
	}
	var scripts []string
	if source[0] != '[' || json.Unmarshal([]byte(source), &scripts) != nil {
		return nil, errors.New("enter a JSON array of JavaScript strings")
	}
	return scripts, nil
}

func (w *workbench) hasDraftScripts() bool {
	for _, editor := range []*requestEditor{&w.preScripts, &w.postScripts} {
		if editor.input.Editor == nil {
			continue
		}
		scripts, err := parseRequestScripts(editor.input.Editor.String())
		if err != nil || len(scripts) != 0 {
			return true
		}
	}
	return false
}

func (w *workbench) drawRequestScripts(f *flex.Frame, canvas *ui.Canvas) {
	label := "Request scripts"
	if w.showScripts {
		label = "Hide request scripts"
	}
	if f.Button(requestScriptsID, label).Clicked() {
		w.showScripts = !w.showScripts
	}
	if w.showScripts {
		w.preScripts.draw(f, canvas, preScriptsEditorID, "Pre-request scripts (JSON array)", "Enter pre-request scripts").Height(92)
		w.postScripts.draw(f, canvas, postScriptsEditorID, "Post-response scripts (JSON array)", "Enter post-response scripts").Height(92)
		if !localTarget && !w.useAgent() {
			f.Wrap("Hosted scripts can only send to configured HTTP URLs.").Muted()
		}
	}
}
