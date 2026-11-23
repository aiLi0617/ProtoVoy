package main

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

var (
	amqpModeID         = tree.ID(page, "amqp-mode")
	amqpBackID         = tree.ID(page, "amqp-back")
	amqpToggleID       = tree.ID(page, "amqp-toggle")
	amqpSettingsID     = tree.ID(page, "amqp-settings")
	amqpExchangeID     = tree.ID(page, "amqp-exchange")
	amqpExchangeTypeID = tree.ID(page, "amqp-exchange-type")
	amqpKeyID          = tree.ID(page, "amqp-key")
	amqpQueueID        = tree.ID(page, "amqp-queue")
	amqpLimitID        = tree.ID(page, "amqp-limit")
	amqpEncodingID     = tree.ID(page, "amqp-encoding")
	amqpContentTypeID  = tree.ID(page, "amqp-content-type")
	amqpDeclareID      = tree.ID(page, "amqp-declare")
	amqpDurableID      = tree.ID(page, "amqp-durable")
	amqpAckID          = tree.ID(page, "amqp-ack")
	amqpUserID         = tree.ID(page, "amqp-user")
	amqpPasswordID     = tree.ID(page, "amqp-password")
	amqpPasswordRefID  = tree.ID(page, "amqp-password-ref")
	amqpServerNameID   = tree.ID(page, "amqp-server-name")
	amqpCAID           = tree.ID(page, "amqp-ca")
)

type amqpPanel struct {
	open, settings                                         bool
	mode, encoding                                         string
	declare, durable, autoAck                              bool
	exchange, exchangeType, key, queue, limit, contentType flex.Field
	username, password, passwordRef, serverName            flex.Field
	ca                                                     requestEditor
	result                                                 result
}

func defaultAMQPRequest() project.Request {
	return project.Request{AMQP: &project.AMQPSettings{Mode: "publish", ExchangeType: "direct",
		Encoding: "text", ContentType: "application/octet-stream", Declare: true, ReceiveLimit: 1}}
}

func (w *workbench) amqpDraft() project.Draft {
	panel := &w.amqp
	limit, _ := strconv.Atoi(strings.TrimSpace(panel.limit.Text()))
	return project.Draft{
		BrokerTimeoutMS: w.brokerTimeout.Text(),
		AMQP: project.AMQPSettings{
			Mode: panel.mode, Encoding: panel.encoding, Declare: panel.declare, Durable: panel.durable, AutoAck: panel.autoAck,
			Exchange: panel.exchange.Text(), ExchangeType: panel.exchangeType.Text(), RoutingKey: panel.key.Text(),
			Queue: panel.queue.Text(), ReceiveLimit: limit, ContentType: panel.contentType.Text(),
			Username: panel.username.Text(), PasswordRef: panel.passwordRef.Text(), ServerName: panel.serverName.Text(), CAPEM: panel.ca.input.Editor.String(),
		},
		Name: w.requestName.Text(), Target: w.target.Text(), Body: w.requestBody.input.Editor.String(),
	}
}

func (w *workbench) applyAMQPDraft(settings project.AMQPSettings) {
	panel := &w.amqp
	panel.open, panel.settings = settings.Mode != "", false
	panel.password.SetText("")
	panel.passwordRef.SetText(settings.PasswordRef)
	panel.result = result{}
	if !panel.open {
		return
	}
	panel.mode, panel.encoding = settings.Mode, settings.Encoding
	panel.declare, panel.durable, panel.autoAck = settings.Declare, settings.Durable, settings.AutoAck
	panel.exchange.SetText(settings.Exchange)
	panel.exchangeType.SetText(settings.ExchangeType)
	panel.key.SetText(settings.RoutingKey)
	panel.queue.SetText(settings.Queue)
	panel.limit.SetText(strconv.Itoa(settings.ReceiveLimit))
	panel.contentType.SetText(settings.ContentType)
	panel.username.SetText(settings.Username)
	panel.serverName.SetText(settings.ServerName)
	panel.ca.input.Editor = text.NewEditor(settings.CAPEM)
}

func (w *workbench) drawAMQP(f *flex.Frame, canvas *ui.Canvas) {
	panel := &w.amqp
	if panel.settings {
		w.drawAMQPSettings(f, canvas)
		return
	}
	f.Row().Gap(8).Items(flex.Center).Children(func() {
		f.Text("AMQP request")
		if w.cancel != nil {
			return
		}
		if f.Button(amqpBackID, "Back to requests").Clicked() && w.canLeaveDraft() {
			w.loadRequest(0, project.Request{})
		}
		if f.Button(amqpSettingsID, "Request options").Clicked() {
			panel.settings = true
		}
	})
	placeholder := "Server-configured AMQP broker"
	if localTarget || w.useAgent() {
		placeholder = "amqp://127.0.0.1:5672"
	}
	f.Row().Gap(8).Children(func() {
		f.Field(&w.requestName, requestNameID, "Request name", "Request name").Grow(1)
		f.Field(&w.target, targetID, "AMQP broker and virtual host", placeholder).Grow(1)
	})
	f.Row().Gap(8).Children(func() {
		f.Field(&panel.exchange, amqpExchangeID, "Exchange", "Exchange (blank for default)").Grow(1)
		f.Field(&panel.key, amqpKeyID, "Routing key", "Routing key").Grow(1)
	})
	if panel.mode == "consume" {
		f.Row().Gap(8).Children(func() {
			f.Field(&panel.queue, amqpQueueID, "Queue", "Queue name").Grow(1)
			f.Field(&panel.limit, amqpLimitID, "Receive limit", "Receive limit").Grow(1)
		})
	}
	if w.cancel == nil {
		f.Row().Gap(8).Children(func() {
			next := "consume"
			if panel.mode == "consume" {
				next = "publish"
			}
			if f.Button(amqpToggleID, "Switch to "+next).Clicked() {
				panel.mode = next
			}
			if panel.mode == "publish" {
				label := "Text"
				if panel.encoding == "base64" {
					label = "Base64"
				}
				if f.Button(amqpEncodingID, label).Clicked() {
					panel.encoding = "base64"
					if label == "Base64" {
						panel.encoding = "text"
					}
				}
			}
		})
	}
	w.drawBrokerTimeout(f)
	if panel.mode == "publish" {
		w.requestBody.draw(f, canvas, requestBodyID, "AMQP message payload", "Enter AMQP message").Height(70)
	}
	f.Row().Gap(8).Children(func() {
		if w.cancel != nil {
			if f.Button(cancelID, "Cancel").Clicked() {
				w.cancel()
			}
			return
		}
		label := "Publish AMQP"
		if panel.mode == "consume" {
			label = "Consume AMQP"
		}
		if w.useAgent() {
			label += " via Local Agent"
		}
		if f.Button(sendID, label).Clicked() {
			w.sendAMQP()
		}
		if f.Button(saveRequestID, "Save request").Clicked() {
			w.saveDraft()
		}
		if f.Button(discardID, "Discard edits").Clicked() {
			w.discardDraft()
		}
	})
	f.Wrap(panel.result.status)
	f.ScrollArea(bodyID).Follow(w.cancel != nil).Grow(1).MinHeight(responseMinHeight).Children(func() { f.Wrap(panel.result.body) })
}

func (w *workbench) drawAMQPSettings(f *flex.Frame, canvas *ui.Canvas) {
	panel := &w.amqp
	if f.Button(amqpBackID, "Back to AMQP request").Clicked() {
		panel.settings = false
	}
	f.Row().Gap(8).Children(func() {
		f.Field(&panel.exchangeType, amqpExchangeTypeID, "Exchange type", "direct, topic, fanout or headers").Grow(1)
		f.Field(&panel.contentType, amqpContentTypeID, "Content type", "application/octet-stream").Grow(1)
	})
	f.Row().Gap(8).Children(func() {
		drawAMQPOption(f, amqpDeclareID, &panel.declare, "Declare topology", "Use existing topology")
		drawAMQPOption(f, amqpDurableID, &panel.durable, "Durable", "Transient")
		if panel.mode == "consume" {
			drawAMQPOption(f, amqpAckID, &panel.autoAck, "Auto ack", "Manual ack")
		}
	})
	if localTarget || w.useAgent() {
		f.Row().Gap(8).Children(func() {
			f.Field(&panel.username, amqpUserID, "AMQP username", "Optional username").Grow(1)
			f.Field(&panel.password, amqpPasswordID, "AMQP password", "Optional password").Grow(1)
		})
		f.Field(&panel.passwordRef, amqpPasswordRefID, "AMQP password reference", "Configured password reference")
		f.Field(&panel.serverName, amqpServerNameID, "AMQPS TLS server name", "Optional TLS server name")
		panel.ca.draw(f, canvas, amqpCAID, "AMQPS private CA certificates", "Optional CA certificates (PEM)").Height(84)
	}
}

func drawAMQPOption(f *flex.Frame, id ui.ID, enabled *bool, on, off string) {
	label := off
	if *enabled {
		label = on
	}
	if f.Button(id, label).Clicked() {
		*enabled = !*enabled
	}
}

func (w *workbench) sendAMQP() {
	draft := w.amqpDraft()
	timeoutMS, err := parseRequestTimeout(draft.BrokerTimeoutMS)
	if err != nil {
		w.amqp.result = result{status: err.Error(), done: true}
		return
	}
	settings := draft.AMQP
	if !settings.Valid() || !settings.HasDestination() {
		w.amqp.result = result{status: "Enter valid AMQP settings and a queue for consumption", done: true}
		return
	}
	options := execution.AMQPOptions{
		TimeoutMS: timeoutMS,
		Mode:      settings.Mode, Exchange: settings.Exchange, ExchangeType: settings.ExchangeType, RoutingKey: settings.RoutingKey,
		Queue: settings.Queue, Payload: draft.Body, Encoding: settings.Encoding, ContentType: settings.ContentType,
		Declare: settings.Declare, Durable: settings.Durable, AutoAck: settings.AutoAck, ReceiveLimit: settings.ReceiveLimit,
		Username: settings.Username, Password: w.amqp.password.Text(), PasswordRef: settings.PasswordRef, ServerName: settings.ServerName, CAPEM: settings.CAPEM,
	}
	if options.Mode == "consume" {
		options.Payload = ""
	}
	target, err := options.ResolveVariables(strings.TrimSpace(draft.Target), w.variableLayers()...)
	if err != nil {
		w.amqp.result = result{status: err.Error(), done: true}
		return
	}
	if target == "" && (localTarget || w.useAgent()) {
		w.amqp.result = result{status: "Enter an AMQP broker", done: true}
		return
	}
	run := runDirectAMQP
	if w.useAgent() {
		run = w.agent.client.RunAMQP
	}
	w.startBrokerRun("AMQP", &w.amqp.result, func(ctx context.Context, connected func(), message func(json.RawMessage)) error {
		return run(ctx, target, options, connected, message)
	})
}
