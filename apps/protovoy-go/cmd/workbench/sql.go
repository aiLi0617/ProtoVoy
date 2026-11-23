package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

var (
	sqlModeID        = tree.ID(page, "sql-mode")
	sqlBackID        = tree.ID(page, "sql-back")
	sqlUserID        = tree.ID(page, "sql-user")
	sqlPasswordID    = tree.ID(page, "sql-password")
	sqlPasswordRefID = tree.ID(page, "sql-password-ref")
	sqlLimitID       = tree.ID(page, "sql-limit")
	sqlTimeoutID     = tree.ID(page, "sql-timeout")
	sqlParametersID  = tree.ID(page, "sql-parameters")
	sqlTxID          = tree.ID(page, "sql-transaction")
)

type sqlPanel struct {
	open, transactional                             bool
	username, password, passwordRef, limit, timeout flex.Field
	parameters                                      requestEditor
	result                                          result
}

func defaultSQLRequest() project.Request {
	return project.Request{SQL: &project.SQLSettings{RowLimit: 500}}
}

func (w *workbench) sqlDraft() project.Draft {
	return project.Draft{
		SQL: true, SQLRowLimit: w.sql.limit.Text(),
		SQLTimeoutMS: w.sql.timeout.Text(),
		SQLSettings: project.SQLSettings{
			Username: w.sql.username.Text(), Transactional: w.sql.transactional,
			PasswordRef: w.sql.passwordRef.Text(),
			Parameters:  w.sql.parameters.input.Editor.String(),
		},
		Name: w.requestName.Text(), Target: w.target.Text(), Body: w.requestBody.input.Editor.String(),
	}
}

func (w *workbench) applySQLDraft(draft project.Draft) {
	w.sql.open = draft.SQL
	w.sql.transactional = draft.SQLSettings.Transactional
	w.sql.username.SetText(draft.SQLSettings.Username)
	w.sql.password.SetText("")
	w.sql.passwordRef.SetText(draft.SQLSettings.PasswordRef)
	w.sql.limit.SetText(draft.SQLRowLimit)
	w.sql.timeout.SetText(draft.SQLTimeoutMS)
	w.sql.parameters.input.Editor = text.NewEditor(draft.SQLSettings.Parameters)
	w.sql.result = result{}
}

func sqlSettings(draft project.Draft) (project.SQLSettings, error) {
	settings := draft.SQLSettings
	settings.RowLimit, _ = strconv.Atoi(strings.TrimSpace(draft.SQLRowLimit))
	timeout, err := parseRequestTimeout(draft.SQLTimeoutMS)
	settings.TimeoutMS = timeout
	return settings, err
}

func (w *workbench) drawSQL(f *flex.Frame, canvas *ui.Canvas) {
	f.Row().Gap(8).Items(flex.Center).Children(func() {
		f.Text("SQL request")
		if w.cancel == nil && f.Button(sqlBackID, "Back to requests").Clicked() && w.canLeaveDraft() {
			w.loadRequest(0, project.Request{})
		}
	})
	placeholder := "Server-configured database"
	if localTarget || w.useAgent() {
		placeholder = "postgres://localhost/database"
	}
	f.Row().Gap(8).Children(func() {
		f.Field(&w.requestName, requestNameID, "Request name", "Request name").Grow(1)
		f.Field(&w.target, targetID, "SQL database URL", placeholder).Grow(2)
	})
	f.Row().Gap(8).Children(func() {
		if localTarget || w.useAgent() {
			f.Field(&w.sql.username, sqlUserID, "SQL username", "Username").Grow(1)
			f.Field(&w.sql.password, sqlPasswordID, "SQL password", "Password for this session").Grow(1)
		}
		f.Field(&w.sql.passwordRef, sqlPasswordRefID, "SQL password reference", "Optional password reference").Grow(1)
	})
	f.Row().Gap(8).Children(func() {
		f.Field(&w.sql.limit, sqlLimitID, "SQL row limit", "Row limit (1–10,000)").Grow(1)
		f.Field(&w.sql.timeout, sqlTimeoutID, "SQL timeout in milliseconds", "Timeout (ms), default 30000").Grow(1)
		label := "Transaction off"
		if w.sql.transactional {
			label = "Transaction on"
		}
		if f.Button(sqlTxID, label).Clicked() {
			w.sql.transactional = !w.sql.transactional
		}
	})
	w.requestBody.draw(f, canvas, requestBodyID, "SQL statement", "SELECT …").Height(82)
	w.sql.parameters.draw(f, canvas, sqlParametersID, "SQL parameters", "JSON array, e.g. [42]").Height(60)
	f.Row().Gap(8).Children(func() {
		if w.cancel != nil {
			if f.Button(cancelID, "Cancel").Clicked() {
				w.cancel()
			}
			return
		}
		label := "Run SQL"
		if w.useAgent() {
			label += " via Local Agent"
		}
		if f.Button(sendID, label).Clicked() {
			w.sendSQL()
		}
		if f.Button(saveRequestID, "Save request").Clicked() {
			w.saveDraft()
		}
		if f.Button(discardID, "Discard edits").Clicked() {
			w.discardDraft()
		}
	})
	f.Wrap(w.sql.result.status)
	f.ScrollArea(bodyID).Grow(1).MinHeight(responseMinHeight).Children(func() { f.Wrap(w.sql.result.body) })
}

func (w *workbench) sendSQL() {
	draft := w.sqlDraft()
	settings, err := sqlSettings(draft)
	if err != nil {
		w.sql.result = result{status: err.Error(), done: true}
		return
	}
	request, err := settings.Request(draft.Body)
	input := execution.SQLBridgeRequest{ExpectedTarget: strings.TrimSpace(draft.Target), TimeoutMS: settings.TimeoutMS, SQLRequest: request}
	if err == nil {
		input, err = input.ResolveVariables(w.variableLayers()...)
	}
	if err != nil {
		w.sql.result = result{status: err.Error(), done: true}
		return
	}
	if input.ExpectedTarget == "" && (localTarget || w.useAgent()) {
		w.sql.result = result{status: "Enter a SQL database URL", done: true}
		return
	}
	credentials := execution.SQLCredentials{
		Username: settings.Username, Password: w.sql.password.Text(), PasswordRef: settings.PasswordRef,
	}
	if (localTarget || w.useAgent()) && credentials.Password != "" && credentials.PasswordRef != "" {
		w.sql.result = result{status: "Use a SQL password reference or a session password", done: true}
		return
	}
	run := runDirectSQL
	if w.useAgent() {
		run = w.agent.client.RunSQL
	}
	ctx, cancel := context.WithCancel(w.agentExecutionContext())
	w.cancel = cancel
	w.sql.result = result{status: "Connecting to SQL database…"}
	go func() {
		defer cancel()
		value := result{status: "SQL completed", done: true}
		err := run(ctx, input, credentials, func() {
			w.publish(result{status: "SQL connected · Running statement…"})
		}, func(message json.RawMessage) {
			var pretty bytes.Buffer
			if len(message) <= execution.PreviewLimit && json.Indent(&pretty, message, "", "  ") == nil && pretty.Len() <= execution.PreviewLimit {
				value.body = pretty.String()
				return
			}
			value.body = strings.ToValidUTF8(string(message[:min(len(message), execution.PreviewLimit)]), "�")
			if len(message) > execution.PreviewLimit {
				value.body += "\n… Preview truncated"
			}
		})
		if ctx.Err() != nil {
			value.status = "SQL cancelled"
		} else if errors.Is(err, context.DeadlineExceeded) {
			value.status = "SQL execution timed out"
		} else if err != nil {
			value.status = err.Error()
		}
		w.publish(value)
	}()
}
