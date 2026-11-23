package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

var (
	grpcModeID             = tree.ID(page, "grpc-mode")
	grpcBackID             = tree.ID(page, "grpc-back")
	grpcServiceID          = tree.ID(page, "grpc-service")
	grpcMethodID           = tree.ID(page, "grpc-method")
	grpcRPCModeID          = tree.ID(page, "grpc-rpc-mode")
	grpcFormatID           = tree.ID(page, "grpc-format")
	grpcImportDescriptorID = tree.ID(page, "grpc-import-descriptor")
	grpcRemoveDescriptorID = tree.ID(page, "grpc-remove-descriptor")
)

type descriptorImport struct {
	data []byte
	err  error
}

type grpcPanel struct {
	open                  bool
	service, method, mode flex.Field
	useJSON, picking      bool
	descriptor, notice    string
	imported              *descriptorImport // protected by workbench.mu
	result                result
}

func (w *workbench) grpcDraft() project.Draft {
	mode := strings.TrimSpace(w.grpc.mode.Text())
	if mode == "" {
		mode = "unary"
	}
	return project.Draft{
		HTTPTimeoutMS: w.httpTimeout.Text(),
		GRPC: project.GRPCSettings{
			Service: strings.TrimSpace(w.grpc.service.Text()), Method: strings.TrimSpace(w.grpc.method.Text()), Mode: mode,
			UseJSON: w.grpc.useJSON, DescriptorSetBase64: w.grpc.descriptor,
		},
		Name: w.requestName.Text(), Target: w.target.Text(),
		Headers: w.headers.input.Editor.String(), Body: w.requestBody.input.Editor.String(),
	}
}

func (w *workbench) drawGRPC(f *flex.Frame, canvas *ui.Canvas) {
	f.Row().Gap(8).Items(flex.Center).Children(func() {
		f.Text("gRPC request")
		if w.cancel == nil && f.Button(grpcBackID, "Back to requests").Clicked() && w.canLeaveDraft() {
			w.loadRequest(0, project.Request{})
			w.invalidator.Invalidate()
		}
	})
	f.Field(&w.requestName, requestNameID, "Request name", "Request name")
	f.Field(&w.httpTimeout, httpTimeoutID, "gRPC timeout in milliseconds", "Timeout (ms), default 30000")
	f.Row().Gap(8).Children(func() {
		f.Field(&w.target, targetID, "gRPC endpoint", "http://127.0.0.1:50051").Grow(1)
		f.Field(&w.grpc.mode, grpcRPCModeID, "gRPC call mode", "unary").Grow(1)
	})
	f.Row().Gap(8).Children(func() {
		f.Field(&w.grpc.service, grpcServiceID, "gRPC service", "Service, e.g. demo.Echo").Grow(1)
		f.Field(&w.grpc.method, grpcMethodID, "gRPC method", "Method, e.g. Watch").Grow(1)
	})
	f.Row().Gap(8).Children(func() {
		if w.cancel != nil || w.grpc.picking {
			return
		}
		label := "Use JSON"
		if w.grpc.useJSON {
			label = "Use Base64"
		}
		if f.Button(grpcFormatID, label).Clicked() {
			w.grpc.useJSON = !w.grpc.useJSON
			w.grpc.notice = ""
		}
		if w.grpc.useJSON || w.grpc.descriptor != "" {
			if f.Button(grpcImportDescriptorID, "Import descriptor").Clicked() {
				w.importGRPCDescriptor()
			}
			if w.grpc.descriptor != "" && f.Button(grpcRemoveDescriptorID, "Remove descriptor").Clicked() {
				w.grpc.descriptor, w.grpc.notice = "", ""
			}
		}
	})
	hint := "Use one Base64 message per line for client and bidirectional streams."
	label, placeholder := "Base64 protobuf messages", "Enter Base64 protobuf bytes"
	if w.grpc.useJSON {
		hint = "Use a JSON array for client and bidirectional streams."
		label, placeholder = "Protobuf JSON messages", "Enter protobuf JSON"
	}
	switch {
	case w.grpc.picking:
		hint = "Opening descriptor…"
	case w.grpc.notice != "":
		hint = w.grpc.notice
	case w.grpc.descriptor != "":
		hint = "Descriptor loaded. " + hint
	case w.grpc.useJSON:
		hint = "Server reflection is used until you import a descriptor. " + hint
	}
	f.Wrap(hint).Muted()
	f.Row().Gap(8).Children(func() {
		w.headers.draw(f, canvas, headersID, "gRPC metadata", "Metadata, one name: value per line").Grow(1).Height(88)
		w.requestBody.draw(f, canvas, requestBodyID, label, placeholder).Grow(1).Height(88)
	})
	f.Row().Gap(8).Children(func() {
		if w.cancel != nil {
			if f.Button(cancelID, "Cancel").Clicked() {
				w.cancel()
			}
			return
		}
		if w.grpc.picking {
			return
		}
		label := "Send gRPC request"
		if w.useAgent() {
			label = "Send gRPC via Local Agent"
		}
		if f.Button(sendID, label).Clicked() {
			w.sendGRPC()
		}
		if f.Button(saveRequestID, "Save request").Clicked() {
			w.saveDraft()
		}
		if f.Button(discardID, "Discard edits").Clicked() {
			w.discardDraft()
		}
	})
	if !localTarget && !w.useAgent() {
		f.Wrap("The browser uses the server-configured gRPC endpoint. Leave the endpoint blank to use it.").Muted()
	}
	f.Wrap(w.grpc.result.status)
	f.ScrollArea(bodyID).Follow(w.cancel != nil).Grow(1).MinHeight(responseMinHeight).Children(func() { f.Wrap(w.grpc.result.body) })
}

func (w *workbench) importGRPCDescriptor() {
	w.grpc.picking = true
	chooseLocalFile(app.OpenFileOptions{
		Title: "Import gRPC descriptor set",
		Filters: []app.FileFilter{
			{Name: "Descriptor set", Pattern: "*.pb"},
			{Name: "Protoset", Pattern: "*.protoset"},
			{Name: "Descriptor", Pattern: "*.desc"},
		},
	}, execution.GRPCDescriptorLimit, func(data []byte, err error) {
		w.mu.Lock()
		w.grpc.imported = &descriptorImport{data: data, err: err}
		w.mu.Unlock()
		w.invalidator.Invalidate()
	})
}

func (w *workbench) finishGRPCDescriptorImport(value *descriptorImport) {
	w.grpc.picking = false
	switch {
	case value.err != nil:
		w.grpc.notice = value.err.Error()
	case value.data == nil:
	case len(value.data) == 0:
		w.grpc.notice = "The descriptor file is empty"
	default:
		w.grpc.descriptor = base64.StdEncoding.EncodeToString(value.data)
		w.grpc.notice = ""
	}
}

func (w *workbench) sendGRPC() {
	draft := w.grpcDraft()
	if strings.TrimSpace(draft.Target) == "" && (w.useAgent() || localTarget || w.collection.selected != 0) {
		w.grpc.result = result{status: "Enter a gRPC endpoint", done: true}
		return
	}
	metadata, err := execution.ParseHeaders(draft.Headers)
	if err != nil {
		w.grpc.result = result{status: err.Error(), done: true}
		return
	}
	timeoutMS, err := parseRequestTimeout(draft.HTTPTimeoutMS)
	if err != nil {
		w.grpc.result = result{status: err.Error(), done: true}
		return
	}
	request := execution.GRPCRequest{
		Target: strings.TrimSpace(draft.Target), Service: draft.GRPC.Service, Method: draft.GRPC.Method,
		Mode: draft.GRPC.Mode, MessageBase64: draft.Body, Metadata: metadata,
		DescriptorSetBase64: draft.GRPC.DescriptorSetBase64,
		TimeoutMS:           timeoutMS,
	}
	if draft.GRPC.UseJSON {
		request.MessageBase64, request.MessageJSON = "", draft.Body
	}
	request, err = request.ResolveVariables(w.variableLayers()...)
	if err == nil {
		err = execution.ValidateGRPCSelection(request.Service, request.Method, request.Mode)
	}
	if err == nil && draft.GRPC.UseJSON && strings.TrimSpace(request.MessageJSON) == "" {
		err = errors.New("enter a gRPC JSON message")
	}
	if err != nil {
		w.grpc.result = result{status: err.Error(), done: true}
		return
	}
	ctx, cancel := context.WithCancel(w.agentExecutionContext())
	w.cancel = cancel
	w.grpc.result = result{status: "Sending gRPC request…"}
	run := runDirectGRPC
	if w.useAgent() {
		run = w.agent.client.RunGRPC
	}
	go func() {
		defer cancel()
		value := result{status: "Waiting for gRPC response…"}
		var entries []string
		total := 0
		err := run(ctx, request, func(status int) {
			value.status = fmt.Sprintf("gRPC streaming · HTTP %d", status)
			w.publish(value)
		}, func(message execution.GRPCMessage) {
			total++
			preview := message.JSONPreview
			if preview == "" {
				preview = base64.StdEncoding.EncodeToString(message.Preview)
				if len(message.Preview) < message.Size {
					preview += "…"
				}
			}
			entry := fmt.Sprintf("Message %d · %d bytes\n%s", total, message.Size, preview)
			const visible = 20
			if len(entries) == visible {
				copy(entries, entries[1:])
				entries = entries[:visible-1]
			}
			entries = append(entries, entry)
			value.body = strings.Join(entries, "\n\n")
			w.publish(value)
		})
		value.done = true
		switch {
		case ctx.Err() != nil || errors.Is(err, context.Canceled):
			value.status = "gRPC cancelled"
		case errors.Is(err, context.DeadlineExceeded):
			value.status = "gRPC execution timed out"
		case err != nil:
			value.status = err.Error()
		default:
			value.status = fmt.Sprintf("gRPC completed · %d messages", total)
		}
		w.publish(value)
	}()
}
