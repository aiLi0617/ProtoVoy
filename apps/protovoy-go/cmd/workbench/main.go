package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/components"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"

	"protovoy.dev/protovoy/internal/agentclient"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

var (
	tree = ui.Tree{}
	page = tree.Feature("request")

	workbenchScrollID  = tree.ID(page, "workbench-scroll")
	targetID           = tree.ID(page, "target")
	methodID           = tree.ID(page, "method")
	headersID          = tree.ID(page, "headers")
	requestBodyID      = tree.ID(page, "request-body")
	protocolID         = tree.ID(page, "protocol")
	operationID        = tree.ID(page, "graphql-operation")
	graphqlVarsID      = tree.ID(page, "graphql-variables")
	subscriptionModeID = tree.ID(page, "graphql-subscription")
	sseModeID          = tree.ID(page, "sse-mode")
	sseRetriesID       = tree.ID(page, "sse-reconnects")
	sseRetryDelayID    = tree.ID(page, "sse-reconnect-delay")
	socketModeID       = tree.ID(page, "websocket-mode")
	socketProtocolsID  = tree.ID(page, "websocket-protocols")
	socketTextID       = tree.ID(page, "websocket-send-text")
	socketBinaryID     = tree.ID(page, "websocket-send-binary")
	socketOptionsID    = tree.ID(page, "websocket-options")
	socketAutomationID = tree.ID(page, "websocket-automation")
	sendID             = tree.ID(page, "send")
	redirectsID        = tree.ID(page, "follow-redirects")
	cancelID           = tree.ID(page, "cancel")
	bodyID             = tree.ID(page, "body")
	responseHeadersID  = tree.ID(page, "response-headers")
	responseFormatID   = tree.ID(page, "response-format")
	responseSaveID     = tree.ID(page, "response-save")
)

const (
	responseMinHeight = 120
	workbenchPadding  = 24
	collectionGap     = 20
)

func workbenchRow(f *flex.Frame, compact bool, minCell float32) flex.Box {
	if compact {
		return f.Grid(minCell).Gap(8)
	}
	return f.Row().Gap(8)
}

type result struct {
	status        string
	body          string
	headers       [][2]string
	headersTime   time.Duration
	previewTime   time.Duration
	previewBytes  int
	formattedJSON string
	rawPreview    []byte
	exportNotice  string
	done          bool
	socketReady   bool
}

type requestEditor struct {
	input  app.TextInput
	layout components.TextareaLayout
}

func newRequestEditor() requestEditor {
	return requestEditor{input: app.TextInput{Editor: text.NewEditor("")}}
}

func (editor *requestEditor) draw(f *flex.Frame, canvas *ui.Canvas, id ui.ID, label, placeholder string) flex.Box {
	model := editor.input.Editor
	selection := model.Selection()
	composition, _ := model.Composition()
	focused := canvas.Focused() == id
	return f.Textarea(components.TextareaSnapshot{
		ID: id, Label: label, Value: model.String(), Placeholder: placeholder,
		Focused: focused, ShowCaret: focused, HideEditorText: true, Layout: &editor.layout,
		Caret:          model.ByteOffset(model.Caret()),
		SelectionStart: model.ByteOffset(selection.Start), SelectionEnd: model.ByteOffset(selection.End),
		CompositionStart: model.ByteOffset(composition.Start), CompositionEnd: model.ByteOffset(composition.End),
	})
}

type workbench struct {
	showResponseHeaders                bool
	showFormattedJSON                  bool
	curlImport                         curlImportPanel
	mock                               mockPanel
	secrets                            secretPanel
	plugins                            pluginPanel
	requestSecret, requestSecretHeader flex.Field
	rpc                                rpcPanel
	agent                              agentPanel
	team                               teamPanel
	schedules                          schedulePanel
	history                            historyPanel
	historySnapshotsEnabled            bool
	historySnapshots                   []project.HistorySnapshot
	grpc                               grpcPanel
	redis                              redisPanel
	mqtt                               mqttPanel
	amqp                               amqpPanel
	sql                                sqlPanel
	kafka                              kafkaPanel
	datagram                           string
	socketFraming                      flex.Field
	socketSendCount                    flex.Field
	socketInterval                     flex.Field
	socketTLS                          bool
	socketServerName                   flex.Field
	socketCA                           requestEditor
	view                               flex.View
	requestName                        flex.Field
	target                             flex.Field
	method                             flex.Field
	followRedirects                    bool
	headers                            requestEditor
	requestBody                        requestEditor
	httpBodyMode                       string
	httpTimeout                        flex.Field
	expectedHTTPStatus                 flex.Field
	responseBodyContains               flex.Field
	brokerTimeout                      flex.Field
	socketTimeout                      flex.Field
	httpBodySource                     string
	httpSourceBody                     string
	httpSourceMode                     string
	httpSourceKind                     string
	showHTTPBodySource                 bool
	httpSourceEditor                   requestEditor
	preScripts, postScripts            requestEditor
	showScripts                        bool
	httpSourceNotice                   string
	multipartBody                      requestEditor
	graphql                            bool
	graphqlOperation                   flex.Field
	graphqlVariables                   requestEditor
	graphqlSubscription                bool
	runningSubscription                bool
	sse                                bool
	sseReconnects                      flex.Field
	sseReconnectDelay                  flex.Field
	runningSSE                         bool
	webSocket                          bool
	socketProtocols                    flex.Field
	socketAutomation                   requestEditor
	socketOptionsOpen                  bool
	socketOutbox                       chan socketFrame
	socketNotice                       string
	collection                         collection
	variables                          variablesPanel
	autosave                           workspaceAutosave
	invalidator                        app.Invalidator
	cancel                             context.CancelFunc
	result                             result

	mu       sync.Mutex
	pending  *result
	imported *projectImport
}

func (w *workbench) prepareRequest() (string, execution.HTTPPayload, error) {
	draft := w.draft()
	headers, err := execution.ParseHeaders(draft.Headers)
	if err != nil {
		return "", execution.HTTPPayload{}, err
	}
	method := strings.TrimSpace(draft.Method)
	if method == "" {
		method = http.MethodGet
	}
	payload := execution.HTTPPayload{
		Method: method, Headers: headers, Body: draft.Body, FollowRedirects: draft.FollowRedirects,
	}
	template := strings.TrimSpace(draft.Target)
	if w.collection.selected != 0 && template == "" {
		return "", execution.HTTPPayload{}, errors.New("enter a URL for the selected request")
	}
	layers := w.variableLayers()
	var target string
	if draft.SSE {
		target, payload, err = execution.PrepareSSE(template, headers, layers...)
	} else if draft.GraphQL {
		document := execution.GraphQLDocument{Query: draft.Body, Variables: draft.GraphQLVariables, OperationName: draft.GraphQLOperation}
		document.TimeoutMS, err = parseRequestTimeout(draft.HTTPTimeoutMS)
		if err == nil {
			target, payload, err = document.Prepare(template, headers, layers...)
		}
	} else {
		payload, err = prepareHTTPBody(draft, payload)
		if err == nil {
			defaults := w.variables.scopes.ActiveEnvironment().HTTPDefaults
			target, payload, err = payload.ResolveHTTPDefaults(template, defaults, layers...)
		}
	}
	if err != nil {
		return "", execution.HTTPPayload{}, err
	}
	if template != "" && strings.TrimSpace(target) == "" {
		return "", execution.HTTPPayload{}, errors.New("request URL resolved to an empty value")
	}
	return target, payload, nil
}

func (w *workbench) variableLayers() []map[string]string {
	scopes := w.variables.scopes
	return []map[string]string{scopes.Global, scopes.ActiveVariables(), scopes.Collection, w.collection.variables}
}

func (w *workbench) send() {
	if w.hasDraftScripts() {
		if w.useAgent() {
			w.sendAgentScripts()
		} else {
			w.sendScriptedHTTP()
		}
		return
	}
	target, payload, err := w.prepareRequest()
	if err != nil {
		w.result = result{status: err.Error(), done: true}
		return
	}
	var options execution.SSEOptions
	reference, header := w.requestSecret.Text(), w.requestSecretHeader.Text()
	if reference != "" && !payload.EventStream && !w.useAgent() {
		w.result = result{status: "Pair a Local Agent to use an HTTP credential", done: true}
		return
	}
	if payload.EventStream {
		options, err = execution.ParseSSEOptions(w.sseReconnects.Text(), w.sseReconnectDelay.Text())
		if err != nil {
			w.result = result{status: err.Error(), done: true}
			return
		}
	}
	ctx, cancel := context.WithCancel(w.agentExecutionContext())
	w.cancel = cancel
	w.runningSSE = payload.EventStream
	w.runningSubscription = false
	w.result = result{status: "Sending…"}
	graphql := w.graphql
	agent, viaAgent := w.agent.client, w.useAgent()
	go func() {
		defer cancel()
		if payload.EventStream {
			open := execute
			if viaAgent {
				open = agent.OpenSSE
			}
			w.publish(readSSE(ctx, target, payload, options, open, w.publish))
		} else if viaAgent && reference != "" {
			w.publish(readAgentHTTPResult(ctx, graphql, w.publish, func(update func(agentclient.Response)) (agentclient.Response, error) {
				return agent.RunHTTPWithSecret(ctx, target, payload, reference, header, update)
			}))
		} else if viaAgent {
			w.publish(readAgentResponse(ctx, agent, target, payload, graphql, w.publish))
		} else {
			w.publish(readResponse(ctx, target, payload, graphql, w.publish))
		}
	}()
}

func (w *workbench) publish(value result) {
	w.mu.Lock()
	w.pending = &value
	w.mu.Unlock()
	w.invalidator.Invalidate()
}

func readResponse(ctx context.Context, target string, payload execution.HTTPPayload, graphql bool, publish func(result)) result {
	return readHTTPResponse(ctx, func() (*execution.HTTPResponse, error) {
		return execute(ctx, target, payload)
	}, graphql, publish)
}

func readHTTPResponse(ctx context.Context, open func() (*execution.HTTPResponse, error), graphql bool, publish func(result)) result {
	value := result{}
	started := time.Now()
	response, err := open()
	if err != nil {
		if ctx.Err() != nil {
			return result{status: "Cancelled", done: true}
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return result{status: "Request timed out", done: true}
		}
		return result{status: err.Error(), done: true}
	}
	defer response.Body.Close()

	value.status = fmt.Sprintf("HTTP %d", response.StatusCode)
	value.headersTime = max(time.Since(started), time.Nanosecond)
	value.headers = execution.PreviewResponseHeaders(response.Headers)
	publish(value)
	body := make([]byte, 0, execution.PreviewLimit+1)
	chunk := make([]byte, 4096)
	reader := io.LimitReader(response.Body, execution.PreviewLimit+1)
	for {
		n, err := reader.Read(chunk)
		if n > 0 {
			body = append(body, chunk[:n]...)
			value.previewBytes = min(len(body), execution.PreviewLimit)
			value.body = strings.ToValidUTF8(string(body[:value.previewBytes]), "�")
		}
		switch {
		case ctx.Err() != nil:
			value.status += " · cancelled"
		case errors.Is(err, context.DeadlineExceeded):
			value.status += " · request timed out"
		case err != nil && err != io.EOF:
			value.status += " · could not finish reading response"
		case len(body) > execution.PreviewLimit:
			value.status += " · preview limited to 8 KiB"
		case err == nil:
			if n > 0 {
				publish(value)
			}
			continue
		case graphql:
			if count := execution.GraphQLErrorCount(body); count != 0 {
				value.status += fmt.Sprintf(" · GraphQL errors: %d", count)
			}
		}
		value.done = true
		value.previewTime = max(time.Since(started), value.headersTime)
		if ctx.Err() == nil && (err == io.EOF || err == nil) {
			value.rawPreview = body[:value.previewBytes]
		}
		if err == io.EOF && ctx.Err() == nil && len(body) <= execution.PreviewLimit {
			value.formattedJSON = formatResponseJSON(value.body)
		}
		return value
	}
}

func (w *workbench) config() app.Config {
	theme := ui.DarkTheme()
	w.redis.password.Secure = true
	w.mqtt.password.Secure = true
	w.amqp.password.Secure = true
	w.sql.password.Secure = true
	w.kafka.password.Secure = true
	w.agent.token.Secure = true
	fields := flex.Fields(&w.requestSecret, &w.requestSecretHeader, &w.secrets.reference, &w.secrets.value, &w.secrets.target, &w.secrets.header, &w.requestName, &w.method, &w.target, &w.httpTimeout, &w.expectedHTTPStatus, &w.responseBodyContains, &w.brokerTimeout, &w.socketTimeout, &w.variables.name, &w.graphqlOperation, &w.sseReconnects, &w.sseReconnectDelay, &w.socketProtocols, &w.socketFraming, &w.socketServerName, &w.socketSendCount, &w.socketInterval,
		&w.plugins.input, &w.plugins.grants,
		&w.plugins.installGrants, &w.mqtt.passwordRef, &w.amqp.passwordRef, &w.redis.passwordRef,
		&w.redis.target, &w.redis.username, &w.redis.password, &w.redis.database, &w.redis.serverName, &w.agent.address, &w.agent.token,
		&w.team.origin, &w.team.email, &w.team.password, &w.team.id,
		&w.team.memberEmail, &w.team.memberName, &w.team.memberPassword,
		&w.team.comments.body,
		&w.grpc.service, &w.grpc.method, &w.grpc.mode, &w.rpc.method, &w.rpc.id, &w.rpc.version, &w.rpc.action,
		&w.mqtt.topic, &w.mqtt.limit, &w.mqtt.clientID, &w.mqtt.username, &w.mqtt.password, &w.mqtt.keepAlive, &w.mqtt.serverName,
		&w.amqp.exchange, &w.amqp.exchangeType, &w.amqp.key, &w.amqp.queue, &w.amqp.limit, &w.amqp.contentType,
		&w.amqp.username, &w.amqp.password, &w.amqp.serverName, &w.sql.username, &w.sql.password, &w.sql.passwordRef, &w.sql.limit, &w.sql.timeout,
		&w.kafka.topic, &w.kafka.key, &w.kafka.partition, &w.kafka.encoding, &w.kafka.group, &w.kafka.offset, &w.kafka.limit,
		&w.kafka.security, &w.kafka.sasl, &w.kafka.username, &w.kafka.password, &w.kafka.passwordRef,
		&w.kafka.caRef, &w.kafka.certRef, &w.kafka.keyRef)
	return app.Config{
		Title: "ProtoVoy", Width: 760, Height: 560, Tree: &tree,
		Invalidator: &w.invalidator,
		TextInput: func(context app.TextInputContext) *app.TextInput {
			switch context.ID {
			case curlImportEditorID:
				return &w.curlImport.editor.input
			case mockEditorID:
				return &w.mock.editor.input
			case headersID:
				return &w.headers.input
			case requestBodyID:
				return &w.requestBody.input
			case multipartBodyID:
				return &w.multipartBody.input
			case httpBodySourceEditorID:
				return &w.httpSourceEditor.input
			case preScriptsEditorID:
				return &w.preScripts.input
			case postScriptsEditorID:
				return &w.postScripts.input
			case socketAutomationID:
				return &w.socketAutomation.input
			case socketCAID:
				return &w.socketCA.input
			case redisCommandsID:
				return &w.redis.commands.input
			case redisCAID:
				return &w.redis.ca.input
			case mqttCAID:
				return &w.mqtt.ca.input
			case amqpCAID:
				return &w.amqp.ca.input
			case sqlParametersID:
				return &w.sql.parameters.input
			case kafkaCAID:
				return &w.kafka.ca.input
			case graphqlVarsID:
				return &w.graphqlVariables.input
			case valuesID:
				return &w.variables.values.input
			default:
				return fields(context)
			}
		},
		OnKey: w.variables.picker.Key,
		OnScroll: func(event ui.ScrollEvent) ui.EventResult {
			if w.variables.open {
				if result := w.variables.picker.Wheel(event); result.Handled {
					return result
				}
			}
			return w.view.Wheel(event)
		},
		OnWake: func() {
			w.mu.Lock()
			pending, imported, paired, history := w.pending, w.imported, w.agent.pending, w.history.pending
			descriptor := w.grpc.imported
			w.grpc.imported = nil
			kafkaTLS := w.kafka.imported
			w.kafka.imported = nil
			environments := w.variables.imported
			w.variables.imported = nil
			w.pending, w.imported, w.agent.pending, w.history.pending = nil, nil, nil, nil
			w.mu.Unlock()
			w.finishAgentWorkspaceLoad()
			w.finishTeamAction()
			w.finishScheduleAction()
			w.finishMockImport()
			w.finishMockRuntime()
			w.finishSecret()
			w.finishPlugins()
			if kafkaTLS != nil {
				w.kafka.finishTLSImport(*kafkaTLS)
			}
			if environments != nil {
				w.variables.finishImport(*environments)
			}
			if pending != nil {
				if w.redis.open {
					w.redis.result = *pending
				} else if w.mqtt.open {
					w.mqtt.result = *pending
				} else if w.amqp.open {
					w.amqp.result = *pending
				} else if w.sql.open {
					w.sql.result = *pending
				} else if w.kafka.mode != "" {
					w.kafka.result = *pending
				} else if w.grpc.open {
					w.grpc.result = *pending
				} else {
					w.result = *pending
				}
				if pending.done {
					w.cancel = nil
					w.socketOutbox = nil
				}
			}
			if imported != nil {
				w.finishImport(*imported)
			}
			if descriptor != nil {
				w.finishGRPCDescriptorImport(descriptor)
			}
			if paired != nil {
				w.finishAgentPair(paired)
			}
			if history != nil {
				w.finishHistory(history)
			}
		},
		Paint: func(canvas *ui.Canvas, size ui.Size) {
			bounds := ui.Rect{Width: size.Width, Height: size.Height}
			compact := size.Width < 560
			stackCollection := size.Width < 800
			collectionWidth := min(230, size.Width/3)
			canvas.Fill(bounds, theme.Background)
			w.view.Draw(canvas, theme, bounds, func(f *flex.Frame) {
				f.ScrollArea(workbenchScrollID).Fill().Pad(workbenchPadding).Gap(12).Children(func() {
					f.Text("ProtoVoy")
					workbenchRow(f, compact, 110).Children(func() {
						if w.agent.open || w.history.open || w.team.open || w.schedules.open {
							if w.cancel == nil && f.Button(agentBackID, "Back to workbench").Clicked() {
								w.agent.token.SetText("")
								w.agent.open = false
								w.team.open = false
								w.schedules.open = false
								w.schedules.password.SetText("")
								w.stopTeamWatch()
								w.team.password.SetText("")
								w.team.memberPassword.SetText("")
								w.history.open = false
								w.history.bodyID, w.history.body = "", agentclient.HistoryResponse{}
								w.invalidator.Invalidate()
							}
							return
						}
						label := "Variables"
						if w.variables.open {
							label = "Back to request"
						}
						if f.Button(variablesID, label).Clicked() {
							w.variables.open = !w.variables.open
							if !w.variables.initialized {
								w.variables.edit(globalVars)
							}
							w.variables.picker.Open = false
						}
						if w.cancel == nil && !w.variables.open && f.Button(agentPanelID, "Local Agent").Clicked() {
							w.agent.open = true
							w.invalidator.Invalidate()
						}
						if w.cancel == nil && !w.variables.open && f.Button(teamPanelID, "Team workspace").Clicked() {
							w.team.open = true
						}
						if w.cancel == nil && !w.variables.open && f.Button(schedulesID, "Schedules").Clicked() {
							w.schedules.open = true
						}
						if w.cancel == nil && !w.variables.open && !w.agent.open && w.agent.client != nil &&
							f.Button(historyPanelID, "History").Clicked() {
							w.history.open = true
							w.loadHistory()
						}
						if w.variables.scopes.Active != "" {
							f.Paragraph("Active environment: "+w.variables.scopes.Active, 1).Muted()
						}
					})
					if w.agent.open {
						w.drawAgent(f)
						return
					}
					if w.schedules.open {
						w.drawSchedules(f)
						return
					}
					if w.team.open {
						w.drawTeam(f)
						return
					}
					if w.history.open {
						w.drawHistory(f)
						return
					}
					if w.curlImport.open {
						w.drawCurlImport(f, canvas, compact)
						return
					}
					if w.mock.open {
						w.drawMock(f, canvas, compact)
						return
					}
					if w.variables.open {
						before, revision := w.variables.scopes.Collection, w.variables.revision
						w.drawVariables(f, canvas)
						if w.collection.project != nil && revision != w.variables.revision && !maps.Equal(before, w.variables.scopes.Collection) {
							w.collection.modified, w.collection.replace = true, false
						}
						if w.autosave.notice != "" {
							f.Wrap(w.autosave.notice).Muted()
						}
						return
					}
					workbenchRow(f, compact, 110).Children(func() {
						if w.collection.picking {
							f.Text("Opening project…")
						} else {
							label := "Import project"
							if w.collection.replace {
								label = "Replace project"
							}
							if f.Button(importID, label).Clicked() {
								w.importProject()
							}
						}
						if f.Button(newRequestID, "New request").Clicked() && w.canLeaveDraft() {
							w.loadRequest(0, project.Request{})
						}
						if f.Button(curlImportID, "Import cURL").Clicked() && w.canLeaveDraft() {
							w.curlImport = curlImportPanel{open: true, editor: newRequestEditor()}
						}
						if f.Button(mockPanelID, "Mock rules").Clicked() {
							if w.mock.editor.input.Editor == nil {
								w.mock.editor = newRequestEditor()
								w.mock.editor.input.Editor = text.NewEditor("[]")
							}
							w.mock.open = true
						}
						if w.collection.project != nil && f.Button(exportID, "Export project").Clicked() {
							w.exportProject()
						}
						if collection := w.collection.project; collection != nil && w.collection.selected > 0 {
							request := collection.Requests[w.collection.selected-1]
							if request.CollectionHTTPProtocol() != "" && f.Button(exportCurlID, "Export saved cURL").Clicked() {
								w.exportCurl(request)
							}
						}
					})
					if w.collection.message != "" {
						f.Wrap(w.collection.message).Muted()
					}
					if w.autosave.notice != "" {
						f.Wrap(w.autosave.notice).Muted()
					}
					if stackCollection && w.collection.project != nil {
						w.drawCollection(f, 0)
					}
					f.Row().Grow(1).Gap(collectionGap).Children(func() {
						requestWidth := size.Width - 2*workbenchPadding
						if !stackCollection && w.collection.project != nil {
							w.drawCollection(f, collectionWidth)
							requestWidth -= collectionWidth + collectionGap
						}
						f.Column().Grow(1).MinWidth(0).Gap(12).Children(func() {
							w.drawRequest(f, canvas, requestWidth < 512)
						})
					})
				})
			})
			w.trackWorkspace()
		},
	}
}

func (w *workbench) drawRequest(f *flex.Frame, canvas *ui.Canvas, compact bool) {
	if w.showHTTPBodySource {
		w.drawHTTPBody(f, canvas, compact)
		return
	}
	if w.rpc.protocol != "" {
		w.drawRPC(f, canvas)
		return
	}
	if w.grpc.open {
		f.Column().Grow(1).MinWidth(0).Gap(8).Children(func() {
			w.drawGRPC(f, canvas)
		})
		return
	}
	if w.redis.open {
		w.drawRedis(f, canvas)
		return
	}
	if w.mqtt.open {
		f.Column().Grow(1).MinWidth(0).Gap(8).Children(func() {
			w.drawMQTT(f, canvas)
		})
		return
	}
	if w.amqp.open {
		f.Column().Grow(1).MinWidth(0).Gap(8).Children(func() {
			w.drawAMQP(f, canvas)
		})
		return
	}
	if w.kafka.mode != "" {
		f.Column().Grow(1).MinWidth(0).Gap(8).Children(func() {
			w.drawKafka(f, canvas)
		})
		return
	}
	if w.sql.open {
		f.Column().Grow(1).MinWidth(0).Gap(8).Children(func() {
			w.drawSQL(f, canvas)
		})
		return
	}
	if w.datagram != "" {
		w.drawDatagram(f, canvas)
		return
	}
	workbenchRow(f, compact, 96).Children(func() {
		if w.cancel == nil {
			w.drawRPCModes(f)
		}
		if f.Button(tcpModeID, "TCP").Clicked() {
			w.datagram = "tcp"
		}
		if f.Button(udpModeID, "UDP").Clicked() {
			w.datagram = "udp"
		}
		if w.cancel == nil && f.Button(redisModeID, "Redis").Clicked() && w.canLeaveDraft() {
			w.loadRequest(0, project.Request{Redis: &project.RedisSettings{Protocol: 2}})
			w.invalidator.Invalidate()
		}
	})
	if w.cancel == nil {
		workbenchRow(f, compact, 96).Children(func() {
			if f.Button(grpcModeID, "gRPC").Clicked() && w.canLeaveDraft() {
				w.loadRequest(0, project.Request{GRPC: &project.GRPCSettings{Mode: "unary"}})
				w.invalidator.Invalidate()
			}
			if f.Button(mqttModeID, "MQTT").Clicked() && w.canLeaveDraft() {
				w.loadRequest(0, defaultMQTTRequest())
				w.invalidator.Invalidate()
			}
			if f.Button(amqpModeID, "AMQP").Clicked() && w.canLeaveDraft() {
				w.loadRequest(0, defaultAMQPRequest())
				w.invalidator.Invalidate()
			}
			if f.Button(sqlModeID, "SQL").Clicked() && w.canLeaveDraft() {
				w.loadRequest(0, defaultSQLRequest())
				w.invalidator.Invalidate()
			}
			if f.Button(kafkaModeID, "Kafka").Clicked() && w.canLeaveDraft() {
				w.loadRequest(0, defaultKafkaRequest())
				w.invalidator.Invalidate()
			}
		})
	}
	f.Field(&w.requestName, requestNameID, "Request name", "Request name")
	workbenchRow(f, compact, 160).Children(func() {
		label := "Switch to GraphQL"
		if w.graphql {
			label = "Switch to HTTP"
		}
		if f.Button(protocolID, label).Clicked() {
			w.graphql = !w.graphql
			w.sse, w.webSocket = false, false
		}
		sseLabel := "Switch to SSE"
		if w.sse {
			sseLabel = "Switch to HTTP"
		}
		if f.Button(sseModeID, sseLabel).Clicked() {
			w.sse = !w.sse
			w.graphql, w.webSocket = false, false
		}
		socketLabel := "WebSocket"
		if w.webSocket {
			socketLabel = "Switch to HTTP"
		}
		if f.Button(socketModeID, socketLabel).Clicked() {
			w.webSocket = !w.webSocket
			w.graphql, w.sse = false, false
		}
		if w.webSocket {
			f.Field(&w.socketProtocols, socketProtocolsID, "WebSocket subprotocols", "Subprotocols (comma-separated)").Grow(1)
		} else if w.sse {
			f.Field(&w.sseReconnects, sseRetriesID, "SSE reconnect attempts", "Reconnect attempts (0–10)").Grow(1)
			f.Field(&w.sseReconnectDelay, sseRetryDelayID, "Initial SSE reconnect delay in milliseconds", "Delay in ms (default 1000)").Grow(1)
		} else if w.graphql {
			f.Field(&w.graphqlOperation, operationID, "GraphQL operation name", "Operation name (optional)").Grow(1)
		} else {
			f.Field(&w.method, methodID, "HTTP method", "GET").Grow(1)
		}
	})
	placeholder := "Server-configured endpoint"
	if localTarget || w.useAgent() {
		placeholder = "http://127.0.0.1:8080/"
		if w.webSocket || (w.graphql && w.graphqlSubscription) {
			placeholder = "ws://127.0.0.1:8080/"
		}
	}
	if w.graphql {
		workbenchRow(f, compact, 192).Children(func() {
			f.Field(&w.target, targetID, "Request URL", placeholder).Grow(1)
			label := "Use subscription"
			if w.graphqlSubscription {
				label = "Use HTTP POST"
			}
			if f.Button(subscriptionModeID, label).Clicked() {
				w.graphqlSubscription = !w.graphqlSubscription
			}
		})
	} else {
		f.Field(&w.target, targetID, "Request URL", placeholder)
	}
	w.headers.draw(f, canvas, headersID, "Request headers", "Content-Type: application/json").Height(84)
	if w.webSocket {
		if w.cancel == nil {
			label := "Connection options"
			if w.socketOptionsOpen {
				label = "Back to manual message"
			}
			if f.Button(socketOptionsID, label).Clicked() {
				w.socketOptionsOpen = !w.socketOptionsOpen
			}
		}
		if w.socketOptionsOpen {
			w.socketAutomation.draw(f, canvas, socketAutomationID, "WebSocket connection options JSON", `{"messages":[{"encoding":"text","data":"hello"}]}`).Height(84)
		} else {
			w.requestBody.draw(f, canvas, requestBodyID, "WebSocket message", "Enter a text or Base64 message").Height(84)
		}
	} else if w.sse {
		f.Text("SSE · GET")
	} else if w.graphql {
		if !w.graphqlSubscription {
			f.Field(&w.httpTimeout, httpTimeoutID, "GraphQL HTTP timeout in milliseconds", "Timeout (ms), default 30000")
			w.drawHTTPAssertions(f)
		}
		workbenchRow(f, compact, 256).Children(func() {
			w.requestBody.draw(f, canvas, requestBodyID, "GraphQL query", "Enter a GraphQL query").Grow(1).Height(84)
			w.graphqlVariables.draw(f, canvas, graphqlVarsID, "GraphQL variables JSON", "GraphQL variables (JSON object)").Grow(1).Height(84)
		})
	} else {
		w.drawHTTPBody(f, canvas, compact)
		if !w.sse {
			w.drawRequestScripts(f, canvas)
		}
	}
	if !w.sse && !w.webSocket && !w.graphqlSubscription && (w.useAgent() || w.requestSecret.Text() != "") {
		f.Field(&w.requestSecret, requestSecretID, "HTTP credential reference", "Bound credential reference")
		f.Field(&w.requestSecretHeader, requestSecretHeaderID, "HTTP credential header", "Bound request header")
	}
	workbenchRow(f, compact, 192).Children(func() {
		if w.cancel == nil {
			if !w.graphql && !w.sse && !w.webSocket {
				label := "Follow redirects off"
				if w.followRedirects {
					label = "Follow redirects on"
				}
				if f.Button(redirectsID, label).Clicked() {
					w.followRedirects = !w.followRedirects
				}
			}
			label := "Send"
			if w.useAgent() {
				label = "Send via Local Agent"
			}
			if w.sse || w.webSocket {
				label = "Connect"
			}
			if (w.sse || w.webSocket) && w.useAgent() {
				label = "Connect via Local Agent"
			}
			if w.graphql && w.graphqlSubscription {
				label = "Subscribe"
				if w.useAgent() {
					label = "Subscribe via Local Agent"
				}
			}
			if f.Button(sendID, label).Clicked() {
				if w.hasDraftScripts() {
					w.send()
				} else if w.webSocket {
					w.startSocket()
				} else if w.graphql && w.graphqlSubscription {
					w.startSubscription()
				} else {
					w.send()
				}
			}
		} else {
			label := "Cancel"
			if w.runningSSE || w.socketOutbox != nil {
				label = "Disconnect"
			}
			if w.runningSubscription {
				label = "Stop subscription"
			}
			if f.Button(cancelID, label).Clicked() {
				w.cancel()
			}
		}
		if f.Button(saveRequestID, "Save request").Clicked() {
			w.saveDraft()
		}
		if f.Button(discardID, "Discard edits").Clicked() {
			w.discardDraft()
		}
		if w.graphql && f.Button(exportGraphQLID, "Export GraphQL").Clicked() {
			w.exportGraphQLRequest()
		}
	})
	if w.webSocket && w.result.socketReady {
		workbenchRow(f, compact, 120).Children(func() {
			if f.Button(socketTextID, "Send text").Clicked() {
				w.sendFrame(websocket.MessageText)
			}
			if f.Button(socketBinaryID, "Send binary").Clicked() {
				w.sendFrame(websocket.MessageBinary)
			}
		})
		if w.socketNotice != "" {
			f.Wrap(w.socketNotice).Muted()
		}
	}
	w.drawHTTPResponse(f, w.runningSSE || w.webSocket || w.runningSubscription)
}

func (w *workbench) drawHTTPResponse(f *flex.Frame, follow bool) {
	f.Wrap(w.result.status)
	if w.result.headersTime > 0 {
		timing := "Headers " + displayHTTPDuration(w.result.headersTime)
		if w.result.previewTime > 0 {
			timing += fmt.Sprintf(" · Preview %s · %d bytes", displayHTTPDuration(w.result.previewTime), w.result.previewBytes)
		}
		f.Wrap(timing).Muted()
	}
	showHeaders := len(w.result.headers) != 0 && w.showResponseHeaders
	body := w.result.body
	if len(w.result.headers) != 0 || w.result.formattedJSON != "" || len(w.result.rawPreview) != 0 {
		f.Grid(145).Gap(8).Children(func() {
			w.drawResponseActions(f, &showHeaders, &body)
		})
	}
	if w.result.exportNotice != "" {
		f.Wrap(w.result.exportNotice).Muted()
	}
	f.ScrollArea(bodyID).Follow(follow && !showHeaders).Grow(1).MinHeight(responseMinHeight).Children(func() {
		if showHeaders {
			for _, header := range w.result.headers {
				f.Wrap(header[0] + ": " + header[1])
			}
			return
		}
		f.Wrap(body)
	})
}

func (w *workbench) drawResponseActions(f *flex.Frame, showHeaders *bool, body *string) {
	if len(w.result.headers) != 0 {
		label := "Response headers"
		if *showHeaders {
			label = "Response body"
		}
		if f.Button(responseHeadersID, label).Clicked() {
			w.showResponseHeaders = !w.showResponseHeaders
			*showHeaders = w.showResponseHeaders
		}
	}
	if !*showHeaders && w.result.formattedJSON != "" {
		label := "Format JSON"
		if w.showFormattedJSON {
			label = "Show raw JSON"
		}
		if f.Button(responseFormatID, label).Clicked() {
			w.showFormattedJSON = !w.showFormattedJSON
		}
		if w.showFormattedJSON {
			*body = w.result.formattedJSON
		}
	}
	if len(w.result.rawPreview) != 0 && f.Button(responseSaveID, "Save preview bytes").Clicked() {
		saved, err := saveExportFile("protovoy-response-preview.bin", "bin", "Binary", "application/octet-stream", w.result.rawPreview)
		if err != nil {
			w.result.exportNotice = err.Error()
		} else if saved {
			w.result.exportNotice = "Response preview saved"
		}
	}
}

func displayHTTPDuration(value time.Duration) string {
	if value < time.Millisecond {
		return "<1 ms"
	}
	return fmt.Sprintf("%d ms", value.Round(time.Millisecond).Milliseconds())
}

func newWorkbench() *workbench {
	return &workbench{
		result: result{status: "Ready"}, headers: newRequestEditor(), requestBody: newRequestEditor(),
		preScripts: newRequestEditor(), postScripts: newRequestEditor(),
		multipartBody:    newRequestEditor(),
		graphqlVariables: newRequestEditor(), socketCA: newRequestEditor(),
		socketAutomation: newRequestEditor(),
	}
}

func main() {
	w := newWorkbench()
	defer w.stopTeamWatch()
	w.restoreWorkspace()
	w.resumeTeamSSO()
	removeExitHook := onWorkspaceExit(w.flushWorkspace)
	defer removeExitHook()
	removeTouchScroll := installTouchScroll()
	defer removeTouchScroll()
	var canvas ui.Canvas
	if err := app.RunCanvas(w.config(), &canvas); err != nil {
		log.Fatal(err)
	}
	if w.cancel != nil {
		w.cancel()
	}
	if w.autosave.timer != nil {
		w.autosave.timer.Stop()
	}
	w.flushWorkspace()
}
