package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

var (
	mqttModeID        = tree.ID(page, "mqtt-mode")
	mqttBackID        = tree.ID(page, "mqtt-back")
	mqttToggleID      = tree.ID(page, "mqtt-toggle")
	mqttTopicID       = tree.ID(page, "mqtt-topic")
	mqttQoSID         = tree.ID(page, "mqtt-qos")
	mqttEncodingID    = tree.ID(page, "mqtt-encoding")
	mqttRetainID      = tree.ID(page, "mqtt-retain")
	mqttLimitID       = tree.ID(page, "mqtt-limit")
	mqttSettingsID    = tree.ID(page, "mqtt-settings")
	mqttClientID      = tree.ID(page, "mqtt-client")
	mqttUserID        = tree.ID(page, "mqtt-user")
	mqttPasswordID    = tree.ID(page, "mqtt-password")
	mqttPasswordRefID = tree.ID(page, "mqtt-password-ref")
	mqttCleanID       = tree.ID(page, "mqtt-clean")
	mqttKeepAliveID   = tree.ID(page, "mqtt-keep-alive")
	mqttServerNameID  = tree.ID(page, "mqtt-server-name")
	mqttCAID          = tree.ID(page, "mqtt-ca")
)

type mqttPanel struct {
	open, settings                                                                 bool
	mode, encoding                                                                 string
	qos                                                                            int
	retain, persistent                                                             bool
	topic, limit, clientID, username, password, passwordRef, keepAlive, serverName flex.Field
	ca                                                                             requestEditor
	result                                                                         result
}

func defaultMQTTRequest() project.Request {
	return project.Request{MQTT: &project.MQTTSettings{Mode: "publish", Encoding: "text", ReceiveLimit: 1, KeepAlive: 30}}
}

func (w *workbench) mqttDraft() project.Draft {
	panel := &w.mqtt
	limit, _ := strconv.Atoi(strings.TrimSpace(panel.limit.Text()))
	keepAlive, err := strconv.Atoi(strings.TrimSpace(panel.keepAlive.Text()))
	if err != nil {
		keepAlive = -1
	}
	return project.Draft{
		BrokerTimeoutMS: w.brokerTimeout.Text(),
		MQTT: project.MQTTSettings{
			Mode: panel.mode, Topic: panel.topic.Text(), Encoding: panel.encoding, QoS: panel.qos, Retain: panel.retain,
			ReceiveLimit: limit, ClientID: panel.clientID.Text(), Username: panel.username.Text(), PasswordRef: panel.passwordRef.Text(),
			PersistentSession: panel.persistent, KeepAlive: keepAlive, ServerName: panel.serverName.Text(), CAPEM: panel.ca.input.Editor.String(),
		},
		Name: w.requestName.Text(), Target: w.target.Text(), Body: w.requestBody.input.Editor.String(),
	}
}

func (w *workbench) applyMQTTDraft(settings project.MQTTSettings) {
	panel := &w.mqtt
	panel.open, panel.settings = settings.Mode != "", false
	panel.password.SetText("")
	panel.passwordRef.SetText(settings.PasswordRef)
	panel.result = result{}
	if !panel.open {
		return
	}
	panel.mode, panel.encoding, panel.qos = settings.Mode, settings.Encoding, settings.QoS
	panel.retain, panel.persistent = settings.Retain, settings.PersistentSession
	panel.topic.SetText(settings.Topic)
	panel.limit.SetText(strconv.Itoa(settings.ReceiveLimit))
	panel.clientID.SetText(settings.ClientID)
	panel.username.SetText(settings.Username)
	panel.keepAlive.SetText(strconv.Itoa(settings.KeepAlive))
	panel.serverName.SetText(settings.ServerName)
	panel.ca.input.Editor = text.NewEditor(settings.CAPEM)
}

func (w *workbench) drawMQTT(f *flex.Frame, canvas *ui.Canvas) {
	panel := &w.mqtt
	direct := localTarget || w.useAgent()
	if panel.settings && direct {
		w.drawMQTTSettings(f, canvas)
		return
	}
	f.Row().Gap(8).Items(flex.Center).Children(func() {
		f.Text("MQTT request")
		if w.cancel != nil {
			return
		}
		if f.Button(mqttBackID, "Back to requests").Clicked() && w.canLeaveDraft() {
			w.loadRequest(0, project.Request{})
		}
		if direct && f.Button(mqttSettingsID, "Connection settings").Clicked() {
			panel.settings = true
		}
	})
	placeholder := "Server-configured MQTT broker"
	if direct {
		placeholder = "mqtt://127.0.0.1:1883"
	}
	f.Row().Gap(8).Children(func() {
		f.Field(&w.requestName, requestNameID, "Request name", "Request name").Grow(1)
		f.Field(&w.target, targetID, "MQTT broker", placeholder).Grow(1)
	})
	f.Row().Gap(8).Children(func() {
		f.Field(&panel.topic, mqttTopicID, "MQTT topic", "Topic, e.g. demo/events").Grow(1)
		if panel.mode == "subscribe" {
			f.Field(&panel.limit, mqttLimitID, "Receive limit", "Receive limit").Grow(1)
		}
	})
	if w.cancel == nil {
		f.Row().Gap(8).Children(func() {
			next := "subscribe"
			if panel.mode == "subscribe" {
				next = "publish"
			}
			if f.Button(mqttToggleID, "Switch to "+next).Clicked() {
				panel.mode = next
			}
			if f.Button(mqttQoSID, fmt.Sprintf("QoS %d", panel.qos)).Clicked() {
				panel.qos = (panel.qos + 1) % 3
			}
			if panel.mode == "publish" {
				label := "Text"
				if panel.encoding == "base64" {
					label = "Base64"
				}
				if f.Button(mqttEncodingID, label).Clicked() {
					panel.encoding = "base64"
					if label == "Base64" {
						panel.encoding = "text"
					}
				}
				label = "Retain off"
				if panel.retain {
					label = "Retain on"
				}
				if f.Button(mqttRetainID, label).Clicked() {
					panel.retain = !panel.retain
				}
			}
		})
	}
	w.drawBrokerTimeout(f)
	if panel.mode == "publish" {
		w.requestBody.draw(f, canvas, requestBodyID, "MQTT message payload", "Enter MQTT message").Height(70)
	}
	f.Row().Gap(8).Children(func() {
		if w.cancel != nil {
			if f.Button(cancelID, "Cancel").Clicked() {
				w.cancel()
			}
			return
		}
		label := "Publish MQTT"
		if panel.mode == "subscribe" {
			label = "Subscribe to MQTT"
		}
		if w.useAgent() {
			label += " via Local Agent"
		}
		if f.Button(sendID, label).Clicked() {
			w.sendMQTT()
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

func (w *workbench) drawMQTTSettings(f *flex.Frame, canvas *ui.Canvas) {
	panel := &w.mqtt
	f.Text("MQTT connection settings")
	if f.Button(mqttBackID, "Back to MQTT request").Clicked() {
		panel.settings = false
	}
	f.Row().Gap(8).Children(func() {
		f.Field(&panel.clientID, mqttClientID, "MQTT client ID", "Optional client ID").Grow(1)
		f.Field(&panel.keepAlive, mqttKeepAliveID, "Keep alive in seconds", "Keep alive seconds").Grow(1)
	})
	f.Row().Gap(8).Children(func() {
		f.Field(&panel.username, mqttUserID, "MQTT username", "Optional username").Grow(1)
		f.Field(&panel.password, mqttPasswordID, "MQTT password", "Optional password").Grow(1)
	})
	label := "Clean session"
	f.Field(&panel.passwordRef, mqttPasswordRefID, "MQTT password reference", "Configured password reference")
	if panel.persistent {
		label = "Persistent session"
	}
	if f.Button(mqttCleanID, label).Clicked() {
		panel.persistent = !panel.persistent
	}
	f.Wrap("For mqtts:// brokers, optionally configure a TLS server name and private CA.").Muted()
	f.Field(&panel.serverName, mqttServerNameID, "MQTT TLS server name", "Optional TLS server name")
	panel.ca.draw(f, canvas, mqttCAID, "MQTT private CA certificates", "Optional CA certificates (PEM)").Height(84)
}

func (w *workbench) sendMQTT() {
	draft := w.mqttDraft()
	timeoutMS, err := parseRequestTimeout(draft.BrokerTimeoutMS)
	if err != nil {
		w.mqtt.result = result{status: err.Error(), done: true}
		return
	}
	settings := draft.MQTT
	if !settings.Valid() || settings.Topic == "" {
		w.mqtt.result = result{status: "Enter a topic and valid MQTT connection settings", done: true}
		return
	}
	options := execution.MQTTOptions{
		TimeoutMS: timeoutMS,
		Mode:      settings.Mode, Topic: settings.Topic, Payload: draft.Body, Encoding: settings.Encoding,
		QoS: settings.QoS, Retain: settings.Retain, ReceiveLimit: settings.ReceiveLimit,
		ClientID: settings.ClientID, Username: settings.Username, Password: w.mqtt.password.Text(), PasswordRef: settings.PasswordRef,
		CleanSession: !settings.PersistentSession, KeepAlive: settings.KeepAlive, ServerName: settings.ServerName, CAPEM: settings.CAPEM,
	}
	if options.Mode == "subscribe" {
		options.Payload = ""
	}
	target, err := options.ResolveVariables(strings.TrimSpace(draft.Target), w.variableLayers()...)
	if err != nil {
		w.mqtt.result = result{status: err.Error(), done: true}
		return
	}
	if target == "" && (localTarget || w.useAgent()) {
		w.mqtt.result = result{status: "Enter an MQTT broker", done: true}
		return
	}
	run := runDirectMQTT
	if w.useAgent() {
		run = w.agent.client.RunMQTT
	}
	w.startBrokerRun("MQTT", &w.mqtt.result, func(ctx context.Context, connected func(), message func(json.RawMessage)) error {
		return run(ctx, target, options, connected, message)
	})
}

var brokerTimeoutID = tree.ID(page, "broker-timeout")

func (w *workbench) drawBrokerTimeout(f *flex.Frame) {
	f.Field(&w.brokerTimeout, brokerTimeoutID, "Execution timeout in milliseconds", "Timeout (ms), default 30000")
}

func (w *workbench) startBrokerRun(protocol string, display *result, run func(context.Context, func(), func(json.RawMessage)) error) {
	ctx, cancel := context.WithCancel(w.agentExecutionContext())
	w.cancel = cancel
	*display = result{status: "Connecting to " + protocol + " broker…"}
	go func() {
		defer cancel()
		value := result{status: protocol + " connected"}
		var entries []string
		total := 0
		err := run(ctx, func() { w.publish(value) }, func(message json.RawMessage) {
			total++
			entry := strings.ToValidUTF8(string(message[:min(len(message), execution.PreviewLimit)]), "�")
			if len(message) > execution.PreviewLimit {
				entry += "…"
			}
			if len(entries) == 20 {
				copy(entries, entries[1:])
				entries = entries[:19]
			}
			entries = append(entries, entry)
			value.body = strings.Join(entries, "\n\n")
			value.status = fmt.Sprintf("%s connected · %d results", protocol, total)
			w.publish(value)
		})
		value.done = true
		switch {
		case ctx.Err() != nil:
			value.status = protocol + " cancelled"
		case errors.Is(err, context.DeadlineExceeded):
			value.status = protocol + " execution timed out"
		case err != nil:
			value.status = err.Error()
		default:
			value.status = fmt.Sprintf("%s completed · %d results", protocol, total)
		}
		w.publish(value)
	}()
}
