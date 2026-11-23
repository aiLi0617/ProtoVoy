package main

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"

	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
	"protovoy.dev/protovoy/internal/execution"
)

var tcpModeID = tree.ID(page, "tcp-mode")
var udpModeID = tree.ID(page, "udp-mode")
var socketBackID = tree.ID(page, "socket-back")
var framingID = tree.ID(page, "socket-framing")
var socketTLSID = tree.ID(page, "socket-tls")
var socketServerNameID = tree.ID(page, "socket-server-name")
var socketCAID = tree.ID(page, "socket-ca")
var socketSendCountID = tree.ID(page, "socket-send-count")
var socketIntervalID = tree.ID(page, "socket-interval")
var socketTimeoutID = tree.ID(page, "socket-timeout")

func (w *workbench) drawDatagram(f *flex.Frame, canvas *ui.Canvas) {
	f.Row().Gap(8).Items(flex.Center).Children(func() {
		f.Text(strings.ToUpper(w.datagram) + " exchange")
		if w.cancel == nil && f.Button(socketBackID, "Back to HTTP").Clicked() {
			w.datagram = ""
		}
	})
	placeholder := "Server-configured socket endpoint"
	if localTarget || w.useAgent() {
		placeholder = "Enter socket host:port"
	}
	f.Row().Gap(8).Children(func() {
		f.Field(&w.requestName, requestNameID, "Request name", "Request name").Grow(1)
		f.Field(&w.target, targetID, "Socket host:port", placeholder).Grow(1)
	})
	f.Row().Gap(8).Children(func() {
		f.Field(&w.method, methodID, "Payload encoding", "text, base64, or hex (default text)").Grow(1)
		if w.datagram == "tcp" {
			f.Field(&w.socketFraming, framingID, "TCP framing", "raw, fixed:8, or delimiter:END").Grow(1)
		}
	})
	f.Row().Gap(8).Children(func() {
		f.Field(&w.socketSendCount, socketSendCountID, "Send count", "Send count (default 1)").Grow(1)
		f.Field(&w.socketInterval, socketIntervalID, "Interval in milliseconds", "Interval in ms (default 0)").Grow(1)
		f.Field(&w.socketTimeout, socketTimeoutID, "Socket timeout in milliseconds", "Timeout (ms), default 30000").Grow(1)
	})
	if w.datagram == "tcp" {
		label := "Enable TLS"
		if w.socketTLS {
			label = "Disable TLS"
		}
		if f.Button(socketTLSID, label).Clicked() {
			w.socketTLS = !w.socketTLS
		}
		if w.socketTLS {
			f.Field(&w.socketServerName, socketServerNameID, "TLS certificate name and SNI", "Server name (defaults to target host)")
			w.socketCA.draw(f, canvas, socketCAID, "Private CA certificates PEM", "Optional private CA certificates (PEM)").Height(84)
		}
	}
	w.requestBody.draw(f, canvas, requestBodyID, "Socket payload", "Enter socket payload").Height(120)
	if w.cancel == nil {
		label := "Send socket payload"
		if w.useAgent() {
			label = "Send socket payload via Local Agent"
		}
		if f.Button(sendID, label).Clicked() {
			w.sendDatagram()
		}
	} else if f.Button(cancelID, "Cancel").Clicked() {
		w.cancel()
	}
	f.Row().Gap(8).Children(func() {
		if f.Button(saveRequestID, "Save request").Clicked() {
			w.saveDraft()
		}
		if f.Button(discardID, "Discard edits").Clicked() {
			w.discardDraft()
		}
	})
	f.Wrap(w.result.status)
	f.ScrollArea(bodyID).Grow(1).MinHeight(responseMinHeight).Children(func() { f.Wrap(w.result.body) })
}

func (w *workbench) sendDatagram() {
	encoding := strings.TrimSpace(w.method.Text())
	if encoding == "GET" {
		encoding = "text"
	}
	payload := execution.SocketPayload{
		Network: w.datagram, ExpectedTarget: strings.TrimSpace(w.target.Text()),
		Data: w.requestBody.input.Editor.String(), Encoding: encoding,
		TLS: w.datagram == "tcp" && w.socketTLS,
	}
	if payload.TLS {
		payload.ServerName = w.socketServerName.Text()
		payload.CAPEM = w.socketCA.input.Editor.String()
	}
	var err error
	payload, err = socketFraming(payload, w.socketFraming.Text())
	if err == nil {
		err = socketSendOptions(&payload, w.socketSendCount.Text(), w.socketInterval.Text())
	}
	if err == nil {
		payload.TimeoutMS, err = parseRequestTimeout(w.socketTimeout.Text())
	}
	if err != nil {
		w.result = result{status: err.Error(), done: true}
		return
	}
	payload, err = payload.ResolveVariables(w.variableLayers()...)
	if err != nil {
		w.result = result{status: err.Error(), done: true}
		return
	}
	ctx, cancel := context.WithCancel(w.agentExecutionContext())
	w.cancel = cancel
	w.result = result{status: "Sending socket payload…"}
	exchange := exchangeDatagram
	if w.useAgent() {
		exchange = w.agent.client.RunSocket
	}
	go func() {
		defer cancel()
		data, err := exchange(ctx, payload)
		value := result{status: "Socket response (Base64)", body: base64.StdEncoding.EncodeToString(data), done: true}
		if errors.Is(err, context.DeadlineExceeded) {
			value.status = "Socket execution timed out"
		} else if err != nil {
			value.status = err.Error()
		}
		if ctx.Err() != nil {
			value.status = "Socket request cancelled"
		}
		w.publish(value)
	}()
}

func socketSendOptions(payload *execution.SocketPayload, count, interval string) error {
	values := [2]*int{&payload.SendCount, &payload.IntervalMS}
	for index, raw := range []string{count, interval} {
		if raw = strings.TrimSpace(raw); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || index == 0 && value == 0 {
				return errors.New("enter a send count of 1 to 10000 and an interval of 0 to 300000 ms")
			}
			*values[index] = value
		}
	}
	if !payload.ValidSendOptions() {
		return errors.New("enter a send count of 1 to 10000 and an interval of 0 to 300000 ms")
	}
	return nil
}

func socketFraming(payload execution.SocketPayload, input string) (execution.SocketPayload, error) {
	if payload.Network != "tcp" {
		return payload, nil
	}
	kind, value, _ := strings.Cut(input, ":")
	payload.Framing = kind
	switch kind {
	case "", "raw":
	case "fixed":
		length, err := strconv.Atoi(value)
		if err != nil || length < 1 || length > execution.SocketResponseLimit {
			return payload, errors.New("enter a fixed response length of 1 to 524288 bytes")
		}
		payload.FixedLength = length
	case "delimiter":
		if value == "" || len(value) > 4096 {
			return payload, errors.New("enter a delimiter of 1 to 4096 bytes")
		}
		payload.Delimiter = value
	default:
		return payload, errors.New("use raw, fixed:8, or delimiter:END")
	}
	return payload, nil
}
