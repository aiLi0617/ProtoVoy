package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

var (
	redisModeID        = tree.ID(page, "redis-mode")
	redisBackID        = tree.ID(page, "redis-back")
	redisTargetID      = tree.ID(page, "redis-target")
	redisCommandsID    = tree.ID(page, "redis-commands")
	redisProtocolID    = tree.ID(page, "redis-protocol")
	redisSettingsID    = tree.ID(page, "redis-settings")
	redisUsernameID    = tree.ID(page, "redis-username")
	redisPasswordID    = tree.ID(page, "redis-password")
	redisPasswordRefID = tree.ID(page, "redis-password-ref")
	redisDatabaseID    = tree.ID(page, "redis-database")
	redisTLSID         = tree.ID(page, "redis-tls")
	redisNameID        = tree.ID(page, "redis-server-name")
	redisCAID          = tree.ID(page, "redis-ca")
)

type redisPanel struct {
	resp3                                                         bool
	open, settings, tls                                           bool
	target, username, password, passwordRef, database, serverName flex.Field
	commands, ca                                                  requestEditor
	result                                                        result
}

func (w *workbench) redisDraft() project.Draft {
	panel := &w.redis
	settings := project.RedisSettings{
		Protocol: 2, Username: panel.username.Text(), PasswordRef: panel.passwordRef.Text(), Database: strings.TrimSpace(panel.database.Text()), TLS: panel.tls,
	}
	if panel.resp3 {
		settings.Protocol = 3
	}
	if panel.tls {
		settings.ServerName = strings.TrimSpace(panel.serverName.Text())
		settings.CAPEM = panel.ca.input.Editor.String()
	}
	return project.Draft{
		BrokerTimeoutMS: w.brokerTimeout.Text(),
		Redis:           settings, Name: w.requestName.Text(), Target: panel.target.Text(), Body: panel.commands.input.Editor.String(),
	}
}

func (w *workbench) applyRedisDraft(draft project.Draft) {
	panel := &w.redis
	panel.open = draft.Redis.Protocol != 0
	panel.resp3 = draft.Redis.Protocol == 3
	panel.settings, panel.tls = false, draft.Redis.TLS
	panel.password.SetText("")
	panel.passwordRef.SetText(draft.Redis.PasswordRef)
	panel.result = result{}
	if !panel.open {
		return
	}
	panel.target.SetText(draft.Target)
	panel.username.SetText(draft.Redis.Username)
	panel.database.SetText(draft.Redis.Database)
	panel.serverName.SetText(draft.Redis.ServerName)
	panel.commands.input.Editor = text.NewEditor(draft.Body)
	panel.ca.input.Editor = text.NewEditor(draft.Redis.CAPEM)
}

func (w *workbench) drawRedis(f *flex.Frame, canvas *ui.Canvas) {
	if w.redis.settings {
		w.drawRedisSettings(f, canvas)
		return
	}
	f.Text("Redis commands")
	if w.cancel == nil {
		f.Row().Gap(8).Children(func() {
			if f.Button(redisBackID, "Back to requests").Clicked() && w.canLeaveDraft() {
				w.loadRequest(0, project.Request{})
				w.invalidator.Invalidate()
			}
			if (localTarget || w.useAgent()) && f.Button(redisSettingsID, "Redis connection settings").Clicked() {
				w.redis.settings = true
				w.invalidator.Invalidate()
			}
		})
	}
	placeholder := "Server-configured Redis endpoint"
	if localTarget || w.useAgent() {
		placeholder = "127.0.0.1:6379"
	}
	f.Field(&w.requestName, requestNameID, "Request name", "Request name")
	f.Field(&w.redis.target, redisTargetID, "Redis host:port", placeholder)
	w.drawBrokerTimeout(f)
	protocol := "RESP2 (switch to RESP3)"
	if w.redis.resp3 {
		protocol = "RESP3 (switch to RESP2)"
	}
	if w.cancel == nil && f.Button(redisProtocolID, protocol).Clicked() {
		w.redis.resp3 = !w.redis.resp3
		w.invalidator.Invalidate()
	}
	f.Wrap("Enter commands as JSON arrays. Keep credentials out of saved commands.").Muted()
	w.redis.commands.draw(f, canvas, redisCommandsID, "Redis commands JSON", `[["PING"]]`).Height(96)
	if w.cancel == nil {
		f.Row().Gap(8).Children(func() {
			label := "Run Redis commands"
			if w.useAgent() {
				label = "Run Redis commands via Local Agent"
			}
			if f.Button(sendID, label).Clicked() {
				w.sendRedis()
			}
			if f.Button(saveRequestID, "Save request").Clicked() {
				w.saveDraft()
			}
			if f.Button(discardID, "Discard edits").Clicked() {
				w.discardDraft()
			}
		})
	} else if f.Button(cancelID, "Cancel").Clicked() {
		w.cancel()
	}
	f.Wrap(w.redis.result.status)
	f.ScrollArea(bodyID).Grow(1).MinHeight(responseMinHeight).Children(func() { f.Wrap(w.redis.result.body) })
}

func (w *workbench) drawRedisSettings(f *flex.Frame, canvas *ui.Canvas) {
	panel := &w.redis
	f.Text("Redis connection settings")
	if f.Button(redisBackID, "Back to Redis commands").Clicked() {
		panel.settings = false
		w.invalidator.Invalidate()
	}
	f.Field(&panel.username, redisUsernameID, "Redis ACL username", "Optional ACL username")
	f.Field(&panel.password, redisPasswordID, "Redis password", "Optional password")
	f.Field(&panel.passwordRef, redisPasswordRefID, "Redis password reference", "Configured password reference")
	f.Field(&panel.database, redisDatabaseID, "Redis database number", "Optional database number")
	label := "Enable verified Redis TLS"
	if panel.tls {
		label = "Disable Redis TLS"
	}
	if f.Button(redisTLSID, label).Clicked() {
		panel.tls = !panel.tls
		w.invalidator.Invalidate()
	}
	if panel.tls {
		f.Field(&panel.serverName, redisNameID, "Redis TLS server name", "Certificate name (defaults to target host)")
		panel.ca.draw(f, canvas, redisCAID, "Redis private CA certificates", "Optional private CA certificates (PEM)").Height(84)
	}
}

func (w *workbench) sendRedis() {
	draft := w.redisDraft()
	timeoutMS, err := parseRequestTimeout(draft.BrokerTimeoutMS)
	if err != nil {
		w.redis.result = result{status: err.Error(), done: true}
		return
	}
	commands, err := execution.ParseRedisCommands(draft.Body)
	if err != nil {
		w.redis.result = result{status: err.Error(), done: true}
		return
	}
	payload := execution.RedisPayload{ExpectedTarget: strings.TrimSpace(draft.Target), Commands: commands, Protocol: draft.Redis.Protocol, TimeoutMS: timeoutMS}
	if err := payload.ResolveVariables(w.variableLayers()...); err != nil {
		w.redis.result = result{status: err.Error(), done: true}
		return
	}
	if (localTarget || w.useAgent()) && payload.ExpectedTarget == "" {
		w.redis.result = result{status: "Enter a Redis host:port", done: true}
		return
	}
	options := execution.RedisOptions{
		Username: draft.Redis.Username, Password: w.redis.password.Text(), PasswordRef: draft.Redis.PasswordRef,
		Database: draft.Redis.Database, TLS: draft.Redis.TLS, ServerName: draft.Redis.ServerName, CAPEM: draft.Redis.CAPEM,
	}
	ctx, cancel := context.WithCancel(w.agentExecutionContext())
	w.cancel = cancel
	w.redis.result = result{status: "Running Redis commands…"}
	exchange := exchangeRedis
	if w.useAgent() {
		exchange = w.agent.client.RunRedis
	}
	go func() {
		defer cancel()
		values, err := exchange(ctx, payload, options)
		value := result{status: "Redis responses (JSON)", done: true}
		switch {
		case ctx.Err() != nil:
			value.status = "Redis request cancelled"
		case errors.Is(err, context.DeadlineExceeded):
			value.status = "Redis execution timed out"
		case err != nil:
			value.status = err.Error()
		default:
			body, err := json.Marshal(values)
			if err != nil {
				value.status = "Could not format Redis responses"
			}
			value.body = string(body)
		}
		w.publish(value)
	}()
}
