package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/text"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

var (
	importID        = tree.ID(page, "import")
	newRequestID    = tree.ID(page, "new-request")
	requestNameID   = tree.ID(page, "request-name")
	saveRequestID   = tree.ID(page, "save-request")
	discardID       = tree.ID(page, "discard")
	exportID        = tree.ID(page, "export")
	exportCurlID    = tree.ID(page, "export-curl")
	exportGraphQLID = tree.ID(page, "export-graphql")
	previousID      = tree.ID(page, "previous")
	nextID          = tree.ID(page, "next")
	collectionItems = tree.Sub(page, tree.Name("collection-requests"))
)

type projectImport struct {
	value *project.Project
	err   error
}

type collection struct {
	project   *project.Project
	picking   bool
	page      int
	message   string
	variables map[string]string
	selected  int // 1-based; zero denotes a new request
	baseline  project.Draft
	modified  bool
	replace   bool
	revision  uint64
}

func (w *workbench) draft() project.Draft {
	if w.kafka.mode != "" {
		return w.kafkaDraft()
	}
	if w.sql.open {
		return w.sqlDraft()
	}
	if w.amqp.open {
		return w.amqpDraft()
	}
	if w.mqtt.open {
		return w.mqttDraft()
	}
	if w.grpc.open {
		return w.grpcDraft()
	}
	if w.redis.open {
		return w.redisDraft()
	}
	draft := project.Draft{
		PreScriptsJSON: w.preScripts.input.Editor.String(), PostScriptsJSON: w.postScripts.input.Editor.String(),
		FollowRedirects:  w.followRedirects && w.rpc.protocol == "" && w.datagram == "" && !w.graphql && !w.sse && !w.webSocket,
		RPC:              w.rpc.settings(),
		SocketNetwork:    w.datagram,
		SocketSendCount:  w.socketSendCount.Text(),
		SocketInterval:   w.socketInterval.Text(),
		SocketFraming:    w.socketFraming.Text(),
		SocketTLS:        w.socketTLS,
		SocketServerName: w.socketServerName.Text(),
		SocketCA:         w.socketCA.input.Editor.String(),
		Name:             w.requestName.Text(), Method: w.method.Text(), Target: w.target.Text(),
		HTTPSecretReference: w.requestSecret.Text(), HTTPSecretHeader: w.requestSecretHeader.Text(),
		Headers: w.headers.input.Editor.String(), Body: w.requestBody.input.Editor.String(),
		GraphQL: w.graphql, GraphQLVariables: w.graphqlVariables.input.Editor.String(),
		GraphQLOperation:    w.graphqlOperation.Text(),
		GraphQLSubscription: w.graphqlSubscription,
		SSE:                 w.sse, SSEReconnects: w.sseReconnects.Text(),
		SSEReconnectDelay: w.sseReconnectDelay.Text(),
		WebSocket:         w.webSocket, WebSocketProtocols: w.socketProtocols.Text(),
		WebSocketAutomation: w.socketAutomation.input.Editor.String(),
	}
	if w.datagram != "" {
		draft.SocketTimeoutMS = w.socketTimeout.Text()
	}
	if w.rpc.protocol != "" || w.graphql && !w.graphqlSubscription {
		draft.HTTPTimeoutMS = w.httpTimeout.Text()
		draft.ExpectedHTTPStatus = w.expectedHTTPStatus.Text()
		draft.ResponseBodyContains = w.responseBodyContains.Text()
	}
	if w.datagram == "" && w.rpc.protocol == "" && !w.sse && !w.graphql && !w.webSocket {
		draft.HTTPBodyMode = w.httpBodyMode
		draft.HTTPTimeoutMS = w.httpTimeout.Text()
		draft.ExpectedHTTPStatus = w.expectedHTTPStatus.Text()
		draft.ResponseBodyContains = w.responseBodyContains.Text()
		if w.httpBodyMode == "multipart" {
			draft.Body = ""
			draft.MultipartJSON = w.multipartBody.input.Editor.String()
		}
		draft.HTTPBodySource = w.currentHTTPBodySource()
	}
	return draft
}

func (w *workbench) canLeaveDraft() bool {
	if w.showHTTPBodySource && w.httpSourceEditor.input.Editor.String() != w.currentHTTPBodySource() {
		w.collection.message = "Apply or discard source edits first"
		return false
	}
	if w.grpc.picking || w.kafka.picking {
		return false
	}
	if (w.redis.open || w.grpc.open || w.mqtt.open || w.amqp.open || w.sql.open || w.kafka.mode != "" || w.rpc.protocol != "") && w.cancel != nil {
		w.collection.message = "Wait for the request or cancel it first"
		return false
	}
	if w.draft() == w.collection.baseline {
		return true
	}
	w.collection.message = "Save request or discard edits first"
	return false
}

func (w *workbench) importProject() {
	if !w.canLeaveDraft() {
		return
	}
	if w.collection.modified && !w.collection.replace {
		w.collection.message = "Export changes or choose Replace project"
		w.collection.replace = true
		return
	}
	w.collection.replace = false
	w.collection.picking = true
	chooseLocalFile(app.OpenFileOptions{
		Title: "Import project",
		Filters: []app.FileFilter{{Name: "JSON", Pattern: "*.json"}, {Name: "HTTP Archive", Pattern: "*.har"},
			{Name: "OpenAPI YAML", Pattern: "*.yaml"}, {Name: "OpenAPI YML", Pattern: "*.yml"}},
	}, project.MaxBytes, func(data []byte, err error) {
		value := projectImport{err: err}
		if data != nil {
			imported, err := project.Read(bytes.NewReader(data))
			value = projectImport{value: &imported, err: err}
		}
		w.mu.Lock()
		w.imported = &value
		w.mu.Unlock()
		w.invalidator.Invalidate()
	})
}

func (w *workbench) finishImport(value projectImport) {
	w.collection.picking = false
	switch {
	case value.err != nil:
		w.collection.message = value.err.Error()
	case value.value != nil:
		if !w.canLeaveDraft() {
			return
		}
		if w.variables.initialized && w.variables.editing == collectionVars && w.variables.dirty() {
			w.collection.message = "Save collection variables or discard their edits before importing"
			return
		}
		w.collection.project = value.value
		w.variables.scopes.Collection = value.value.CollectionVariables
		if w.variables.initialized && w.variables.editing == collectionVars {
			w.variables.edit(collectionVars)
		}
		w.collection.revision++
		w.collection.page = 0
		w.collection.modified = false
		w.loadRequest(0, project.Request{})
		w.collection.message = fmt.Sprintf("Imported %d requests", len(value.value.Requests))
	}
}

func (w *workbench) loadRequest(selected int, request project.Request) {
	w.requestSecret.SetText("")
	w.requestSecretHeader.SetText("")
	draft := project.Draft{
		PreScriptsJSON: formatRequestScripts(request.PreScripts), PostScriptsJSON: formatRequestScripts(request.PostScripts),
		FollowRedirects:     request.FollowRedirects,
		HTTPSecretReference: request.HTTPSecretReference, HTTPSecretHeader: request.HTTPSecretHeader,
		Name: request.Name, Target: request.URL, Method: request.Method,
		Headers: request.HeaderText(), Body: request.Body,
	}
	if len(request.Multipart) != 0 {
		data, _ := json.MarshalIndent(request.Multipart, "", "  ")
		draft.HTTPBodyMode, draft.MultipartJSON = "multipart", string(data)
	} else if request.BodyEncoding == "base64" {
		draft.HTTPBodyMode = "base64"
	}
	draft.HTTPBodySource = request.BodySource
	if request.HTTPTimeoutMS != 0 {
		draft.HTTPTimeoutMS = strconv.Itoa(request.HTTPTimeoutMS)
	}
	draft.ResponseBodyContains = request.ResponseBodyContains
	if request.ExpectedHTTPStatus != 0 {
		draft.ExpectedHTTPStatus = strconv.Itoa(request.ExpectedHTTPStatus)
	}
	if request.BrokerTimeoutMS != 0 {
		draft.BrokerTimeoutMS = strconv.Itoa(request.BrokerTimeoutMS)
	}
	if request.GraphQL != nil {
		if request.GraphQL.TimeoutMS != 0 {
			draft.HTTPTimeoutMS = strconv.Itoa(request.GraphQL.TimeoutMS)
		}
		draft.GraphQL = true
		draft.Body = request.GraphQL.Query
		draft.GraphQLVariables = request.GraphQL.Variables
		draft.GraphQLOperation = request.GraphQL.OperationName
		draft.GraphQLSubscription = request.GraphQL.Subscription
	}
	if request.SSE != nil {
		draft.SSE = true
		if request.SSE.Reconnects > 0 {
			draft.SSEReconnects = strconv.Itoa(request.SSE.Reconnects)
		}
		if request.SSE.ReconnectDelayMS != nil {
			draft.SSEReconnectDelay = strconv.Itoa(*request.SSE.ReconnectDelayMS)
		}
	}
	if request.WebSocket != nil {
		draft.WebSocket = true
		draft.WebSocketProtocols = request.WebSocket.Subprotocols
		if data, err := json.MarshalIndent(request.WebSocket.WebSocketAutomation, "", "  "); err == nil && string(data) != "{}" {
			draft.WebSocketAutomation = string(data)
		}
	}
	if request.Socket != nil {
		if request.Socket.TimeoutMS != 0 {
			draft.SocketTimeoutMS = strconv.Itoa(request.Socket.TimeoutMS)
		}
		draft.SocketNetwork = request.Socket.Network
		if request.Socket.SendCount != 0 {
			draft.SocketSendCount = strconv.Itoa(request.Socket.SendCount)
		}
		if request.Socket.IntervalMS != 0 {
			draft.SocketInterval = strconv.Itoa(request.Socket.IntervalMS)
		}
		draft.SocketTLS = request.Socket.TLS
		draft.SocketServerName = request.Socket.ServerName
		draft.SocketCA = request.Socket.CAPEM
		draft.Method = request.Socket.Encoding
		draft.Body = request.Socket.Data
		draft.SocketFraming = request.Socket.Framing
		if request.Socket.Framing == "fixed" {
			draft.SocketFraming += ":" + strconv.Itoa(request.Socket.FixedLength)
		}
		if request.Socket.Framing == "delimiter" {
			draft.SocketFraming += ":" + request.Socket.Delimiter
		}
	}
	if request.Redis != nil {
		draft.Redis = *request.Redis
	}
	if request.MQTT != nil {
		draft.MQTT = *request.MQTT
	}
	if request.AMQP != nil {
		draft.AMQP = *request.AMQP
	}
	if request.SQL != nil {
		draft.SQL, draft.SQLSettings = true, *request.SQL
		draft.SQLRowLimit = strconv.Itoa(request.SQL.RowLimit)
		if request.SQL.TimeoutMS != 0 {
			draft.SQLTimeoutMS = strconv.Itoa(request.SQL.TimeoutMS)
		}
	}
	if request.Kafka != nil {
		draft.Kafka = *request.Kafka
	}
	if request.RPC != nil {
		draft.RPC = *request.RPC
		draft.RPC.TimeoutMS = 0
		if request.RPC.TimeoutMS != 0 {
			draft.HTTPTimeoutMS = strconv.Itoa(request.RPC.TimeoutMS)
		}
	}
	if request.GRPC != nil {
		draft.GRPC = *request.GRPC
		draft.GRPC.TimeoutMS = 0
		if request.GRPC.TimeoutMS != 0 {
			draft.HTTPTimeoutMS = strconv.Itoa(request.GRPC.TimeoutMS)
		}
		if draft.GRPC.Mode == "" {
			draft.GRPC.Mode = "unary"
		}
	}
	w.applyDraft(draft)
	w.collection.variables = request.Variables
	w.collection.selected = selected
	w.collection.baseline = w.draft()
}

func (w *workbench) applyDraft(draft project.Draft) {
	w.preScripts.input.Editor = text.NewEditor(draft.PreScriptsJSON)
	w.postScripts.input.Editor = text.NewEditor(draft.PostScriptsJSON)
	w.showScripts = false
	w.requestSecret.SetText(draft.HTTPSecretReference)
	w.requestSecretHeader.SetText(draft.HTTPSecretHeader)
	w.rpc.protocol = draft.RPC.Protocol
	w.rpc.method.SetText(draft.RPC.Method)
	w.rpc.id.SetText(draft.RPC.ID)
	w.rpc.version.SetText(draft.RPC.Version)
	w.rpc.action.SetText(draft.RPC.Action)
	w.applyRedisDraft(draft)
	w.applyMQTTDraft(draft.MQTT)
	w.applyAMQPDraft(draft.AMQP)
	w.applySQLDraft(draft)
	w.applyKafkaDraft(draft.Kafka)
	w.grpc.open = draft.GRPC.Mode != ""
	w.grpc.service.SetText(draft.GRPC.Service)
	w.grpc.method.SetText(draft.GRPC.Method)
	w.grpc.mode.SetText(draft.GRPC.Mode)
	w.grpc.useJSON, w.grpc.descriptor = draft.GRPC.UseJSON, draft.GRPC.DescriptorSetBase64
	w.grpc.notice = ""
	w.grpc.result = result{}
	w.datagram = draft.SocketNetwork
	w.socketFraming.SetText(draft.SocketFraming)
	w.socketSendCount.SetText(draft.SocketSendCount)
	w.socketInterval.SetText(draft.SocketInterval)
	w.socketTimeout.SetText(draft.SocketTimeoutMS)
	w.socketTLS = draft.SocketTLS
	w.socketServerName.SetText(draft.SocketServerName)
	w.socketCA.input.Editor = text.NewEditor(draft.SocketCA)
	w.requestName.SetText(draft.Name)
	w.target.SetText(draft.Target)
	w.method.SetText(draft.Method)
	w.followRedirects = draft.FollowRedirects
	w.headers.input.Editor = text.NewEditor(draft.Headers)
	w.requestBody.input.Editor = text.NewEditor(draft.Body)
	w.httpBodyMode = draft.HTTPBodyMode
	w.httpTimeout.SetText(draft.HTTPTimeoutMS)
	w.expectedHTTPStatus.SetText(draft.ExpectedHTTPStatus)
	w.responseBodyContains.SetText(draft.ResponseBodyContains)
	w.brokerTimeout.SetText(draft.BrokerTimeoutMS)
	w.multipartBody.input.Editor = text.NewEditor(draft.MultipartJSON)
	w.httpBodySource, w.httpSourceMode, w.showHTTPBodySource = draft.HTTPBodySource, draft.HTTPBodyMode, false
	w.httpSourceBody = draft.Body
	if draft.HTTPBodyMode == "multipart" {
		w.httpSourceBody = draft.MultipartJSON
	}
	w.graphql = draft.GraphQL
	w.graphqlVariables.input.Editor = text.NewEditor(draft.GraphQLVariables)
	w.graphqlOperation.SetText(draft.GraphQLOperation)
	w.graphqlSubscription = draft.GraphQLSubscription
	w.sse = draft.SSE
	w.sseReconnects.SetText(draft.SSEReconnects)
	w.sseReconnectDelay.SetText(draft.SSEReconnectDelay)
	w.webSocket = draft.WebSocket
	w.socketProtocols.SetText(draft.WebSocketProtocols)
	w.socketAutomation.input.Editor = text.NewEditor(draft.WebSocketAutomation)
	w.socketOptionsOpen = false
}

func (w *workbench) discardDraft() {
	state := &w.collection
	if state.selected == 0 {
		if state.baseline.Redis.Protocol != 0 || state.baseline.GRPC.Mode != "" || state.baseline.MQTT.Mode != "" || state.baseline.AMQP.Mode != "" || state.baseline.RPC.Protocol != "" || state.baseline.SQL || state.baseline.Kafka.Mode != "" {
			w.applyDraft(state.baseline)
		} else {
			w.loadRequest(0, project.Request{})
		}
	} else {
		w.loadRequest(state.selected, state.project.Requests[state.selected-1])
	}
	state.message = "Edits discarded"
}

func (w *workbench) saveDraft() {
	state := &w.collection
	input := w.draft()
	target := strings.TrimSpace(input.Target)
	isRedis := input.Redis.Protocol != 0
	isGRPC := input.GRPC.Mode != ""
	isMQTT := input.MQTT.Mode != ""
	isAMQP := input.AMQP.Mode != ""
	isKafka := input.Kafka.Mode != ""
	if target == "" && ((!isRedis && !isMQTT && !isAMQP && !input.SQL && !isKafka) || localTarget || w.useAgent()) {
		state.message = "Enter a URL to save this request"
		return
	}
	request := project.Request{}
	if state.selected != 0 {
		request = state.project.Requests[state.selected-1]
	} else {
		request.Variables = state.variables
	}
	if isRedis {
		if err := input.Redis.CheckPersistence(target, input.Body); err != nil {
			state.message = err.Error()
			return
		}
	}
	if isGRPC {
		if err := execution.ValidateGRPCSelection(input.GRPC.Service, input.GRPC.Method, input.GRPC.Mode); err != nil {
			state.message = err.Error()
			return
		}
		if err := input.GRPC.ValidatePayload(input.Body); err != nil {
			state.message = err.Error()
			return
		}
	}
	if isMQTT && (!input.MQTT.Valid() || strings.TrimSpace(input.MQTT.Topic) == "") {
		state.message = "Enter a topic and valid MQTT connection settings"
		return
	}
	if isAMQP && (!input.AMQP.Valid() || !input.AMQP.HasDestination()) {
		state.message = "Enter valid AMQP settings and a queue for consumption"
		return
	}
	if state.selected == 0 || input.Headers != state.baseline.Headers {
		pairs, err := execution.ParseHeaders(input.Headers)
		if err != nil {
			state.message = err.Error()
			return
		}
		request.Headers = make(map[string]string, len(pairs))
		seen := make(map[string]bool, len(pairs))
		for _, pair := range pairs {
			key := strings.ToLower(pair[0])
			if seen[key] {
				state.message = "Portable projects require one value per header name"
				return
			}
			seen[key] = true
			request.Headers[pair[0]] = pair[1]
		}
	}
	request.Name = strings.TrimSpace(input.Name)
	if request.Name == "" {
		request.Name = "Untitled request"
	}
	request.Method = strings.TrimSpace(input.Method)
	if request.Method == "" {
		request.Method = http.MethodGet
	}
	request.URL = target
	body, err := prepareHTTPBody(input, execution.HTTPPayload{Body: input.Body})
	if err != nil {
		state.message = err.Error()
		return
	}
	request.Body, request.BodyEncoding, request.Multipart = body.Body, body.BodyEncoding, body.Multipart
	request.BodySource = input.HTTPBodySource
	request.HTTPTimeoutMS = body.TimeoutMS
	request.BrokerTimeoutMS, err = parseRequestTimeout(input.BrokerTimeoutMS)
	if err != nil {
		state.message = err.Error()
		return
	}
	if err := request.ValidateHTTPBody(); err != nil {
		state.message = err.Error()
		return
	}
	request.FollowRedirects = input.FollowRedirects
	request.GraphQL = nil
	request.SSE = nil
	request.WebSocket = nil
	request.Socket = nil
	request.Redis = nil
	request.GRPC = nil
	request.MQTT = nil
	request.AMQP = nil
	request.SQL = nil
	request.Kafka = nil
	request.RPC = nil
	switch {
	case input.RPC.Valid():
		request.Method = ""
		input.RPC.TimeoutMS, request.HTTPTimeoutMS = request.HTTPTimeoutMS, 0
		request.RPC = &input.RPC
	case isGRPC:
		request.Method = ""
		input.GRPC.TimeoutMS, request.HTTPTimeoutMS = request.HTTPTimeoutMS, 0
		request.GRPC = &input.GRPC
	case isRedis:
		request.Method, request.Headers = "", nil
		request.Redis = &input.Redis
	case isMQTT:
		request.Method, request.Headers = "", nil
		request.MQTT = &input.MQTT
	case isAMQP:
		request.Method, request.Headers = "", nil
		request.AMQP = &input.AMQP
	case input.SQL:
		settings, err := sqlSettings(input)
		if err != nil {
			state.message = err.Error()
			return
		}
		if err := settings.Validate(target, input.Body); err != nil {
			state.message = err.Error()
			return
		}
		request.Method, request.Headers, request.SQL = "", nil, &settings
	case isKafka:
		if err := input.Kafka.Validate(target, input.Body); err != nil {
			state.message = err.Error()
			return
		}
		request.Method, request.Headers, request.Kafka = "", nil, &input.Kafka
	case input.SocketNetwork != "":
		encoding := strings.TrimSpace(input.Method)
		if encoding == "" || encoding == "GET" {
			encoding = "text"
		}
		if encoding != "text" && encoding != "base64" && encoding != "hex" {
			state.message = "Select text, base64, or hex encoding"
			return
		}
		request.Method = ""
		payload, err := socketFraming(execution.SocketPayload{Network: input.SocketNetwork, Encoding: encoding, Data: input.Body}, input.SocketFraming)
		if err == nil {
			err = socketSendOptions(&payload, input.SocketSendCount, input.SocketInterval)
		}
		if err == nil {
			payload.TimeoutMS, err = parseRequestTimeout(input.SocketTimeoutMS)
		}
		if err != nil {
			state.message = err.Error()
			return
		}
		payload.TLS = input.SocketNetwork == "tcp" && input.SocketTLS
		if payload.TLS {
			payload.ServerName = input.SocketServerName
			payload.CAPEM = input.SocketCA
		}
		request.Socket = &payload
	case input.GraphQL:
		request.Method, request.Body = "", ""
		request.GraphQL = &execution.GraphQLDocument{
			Query: input.Body, Variables: input.GraphQLVariables, OperationName: input.GraphQLOperation,
			Subscription: input.GraphQLSubscription,
			TimeoutMS:    request.HTTPTimeoutMS,
		}
		if err := project.ValidateGraphQLTimeout(*request.GraphQL); err != nil {
			state.message = err.Error()
			return
		}
		request.HTTPTimeoutMS = 0
	case input.SSE:
		options, err := execution.ParseSSEOptions(input.SSEReconnects, input.SSEReconnectDelay)
		if err != nil {
			state.message = err.Error()
			return
		}
		request.Method, request.Body = "", ""
		request.SSE = &options
	case input.WebSocket:
		request.Method = ""
		automation, err := parseWebSocketAutomation(input.WebSocketAutomation)
		if err != nil {
			state.message = err.Error()
			return
		}
		if _, err := automation.Frames(); err != nil {
			state.message = err.Error()
			return
		}
		request.WebSocket = &execution.WebSocketOptions{Subprotocols: input.WebSocketProtocols, WebSocketAutomation: automation}
	}
	request.ExpectedHTTPStatus = 0
	request.HTTPSecretReference, request.HTTPSecretHeader = "", ""
	if request.CollectionHTTPProtocol() == "http" || request.CollectionHTTPProtocol() == "graphql" {
		if input.HTTPSecretReference != "" || input.HTTPSecretHeader != "" {
			if input.HTTPSecretReference == "" || !execution.ValidPasswordReference(input.HTTPSecretReference) || input.HTTPSecretHeader == "" || input.FollowRedirects {
				state.message = "HTTP credentials require a reference, a header, and redirects off"
				return
			}
			request.HTTPSecretReference, request.HTTPSecretHeader = input.HTTPSecretReference, input.HTTPSecretHeader
		}
	}
	request.ResponseBodyContains = ""
	if request.CollectionHTTPProtocol() != "" {
		if len(input.ResponseBodyContains) > project.MaxResponseBodyAssertionBytes {
			state.message = "Enter expected response text within 4 KiB"
			return
		}
		request.ResponseBodyContains = input.ResponseBodyContains
	}
	if expected := strings.TrimSpace(input.ExpectedHTTPStatus); expected != "" && request.CollectionHTTPProtocol() != "" {
		status, err := strconv.Atoi(expected)
		if err != nil || status < 200 || status > 599 {
			state.message = "Enter an expected HTTP status from 200 to 599"
			return
		}
		request.ExpectedHTTPStatus = status
	}
	pre, err := parseRequestScripts(input.PreScriptsJSON)
	if err != nil {
		state.message = "Pre-request scripts: " + err.Error()
		return
	}
	post, err := parseRequestScripts(input.PostScriptsJSON)
	if err != nil {
		state.message = "Post-response scripts: " + err.Error()
		return
	}
	if request.CollectionHTTPProtocol() == "http" {
		request.PreScripts, request.PostScripts = pre, post
	}
	if err := request.ValidateScripts(); err != nil {
		state.message = err.Error()
		return
	}
	if state.project == nil {
		state.project = &project.Project{Name: "ProtoVoy project"}
	}
	if state.selected == 0 {
		if err := state.project.Append(request); err != nil {
			state.message = err.Error()
			return
		}
		state.selected = len(state.project.Requests)
		state.page = (state.selected - 1) / collectionPageSize
	} else {
		state.project.Requests[state.selected-1] = request
	}
	password, redisResult, grpcResult := w.redis.password.Text(), w.redis.result, w.grpc.result
	mqttPassword, mqttResult := w.mqtt.password.Text(), w.mqtt.result
	amqpPassword, amqpResult := w.amqp.password.Text(), w.amqp.result
	sqlPassword, sqlResult := w.sql.password.Text(), w.sql.result
	kafkaPassword, kafkaCA, kafkaResult := w.kafka.password.Text(), w.kafka.ca.input.Editor, w.kafka.result
	kafkaCertificate, kafkaPrivateKey := w.kafka.certificate, w.kafka.privateKey
	w.loadRequest(state.selected, request)
	if isRedis {
		w.redis.password.SetText(password)
		w.redis.result = redisResult
	}
	if isGRPC {
		w.grpc.result = grpcResult
	}
	if isMQTT {
		w.mqtt.password.SetText(mqttPassword)
		w.mqtt.result = mqttResult
	}
	if isAMQP {
		w.amqp.password.SetText(amqpPassword)
		w.amqp.result = amqpResult
	}
	if input.SQL {
		w.sql.password.SetText(sqlPassword)
		w.sql.result = sqlResult
	}
	if isKafka {
		w.kafka.password.SetText(kafkaPassword)
		w.kafka.ca.input.Editor = kafkaCA
		w.kafka.certificate, w.kafka.privateKey = kafkaCertificate, kafkaPrivateKey
		w.kafka.result = kafkaResult
	}
	state.revision++
	state.modified, state.replace = true, false
	state.message = "Request saved in collection"
}

func (w *workbench) exportProject() {
	if !w.canLeaveDraft() {
		return
	}
	value := *w.collection.project
	value.CollectionVariables = w.variables.scopes.Collection
	data, err := project.Encode(value)
	if err != nil {
		w.collection.message = err.Error()
		return
	}
	saved, err := saveProjectFile("protovoy-project.json", data)
	if err != nil {
		w.collection.message = err.Error()
	} else if saved {
		w.collection.modified, w.collection.replace = false, false
		w.collection.message = "Project file exported"
	}
}

func (w *workbench) exportCurl(request project.Request) {
	snippet, err := request.CurlSnippet(w.variables.scopes.Collection)
	if err != nil {
		w.collection.message = err.Error()
		return
	}
	saved, err := saveExportFile("protovoy-curl.txt", "txt", "Text", "text/plain;charset=utf-8", []byte(snippet+"\n"))
	if err != nil {
		w.collection.message = err.Error()
	} else if saved {
		w.collection.message = "cURL command exported"
	}
}

func (w *workbench) exportGraphQLRequest() {
	input := w.draft()
	headers, err := execution.ParseHeaders(input.Headers)
	if err != nil {
		w.collection.message = err.Error()
		return
	}
	timeoutMS, err := parseRequestTimeout(input.HTTPTimeoutMS)
	if err != nil {
		w.collection.message = err.Error()
		return
	}
	data, err := project.EncodeGraphQL(input.Name, strings.TrimSpace(input.Target), headers, execution.GraphQLDocument{
		Query: input.Body, Variables: input.GraphQLVariables, OperationName: input.GraphQLOperation,
		Subscription: input.GraphQLSubscription,
		TimeoutMS:    timeoutMS,
	}, w.collection.variables)
	if err != nil {
		w.collection.message = err.Error()
		return
	}
	saved, err := saveProjectFile("protovoy-graphql.json", data)
	if err != nil {
		w.collection.message = err.Error()
	} else if saved {
		w.collection.message = "GraphQL request exported"
	}
}

const collectionPageSize = 6

func (w *workbench) drawCollection(f *flex.Frame, width float32) {
	state := &w.collection
	collection := f.Column().Gap(8)
	if width > 0 {
		collection.Width(width).Shrink(0)
	}
	collection.Children(func() {
		f.Wrap(state.project.Name)
		requests := state.project.Requests
		folders := state.project.EmptyFolders
		total := len(requests) + len(folders)
		start := state.page * collectionPageSize
		end := min(start+collectionPageSize, total)
		for index := start; index < min(end, len(requests)); index++ {
			request := requests[index]
			if len(request.CollectionPath) != 0 {
				f.Paragraph(strings.Join(request.CollectionPath, " / "), 1).Muted()
			}
			if f.Button(collectionItems.At(index), request.Name).MaxWidthPercent(100).Clicked() && w.canLeaveDraft() {
				w.loadRequest(index+1, request)
			}
		}
		for index := max(start, len(requests)); index < end; index++ {
			f.Wrap(strings.Join(folders[index-len(requests)], " / ") + " (empty folder)").Muted()
		}
		f.Row().Gap(4).Children(func() {
			if start > 0 && f.Button(previousID, "Previous").Clicked() {
				state.page--
			}
			if end < total && f.Button(nextID, "Next").Clicked() {
				state.page++
			}
		})
	})
}
