package project

import (
	"encoding/json"
	"errors"
	"strings"
)

type Draft struct {
	PreScriptsJSON       string        `json:"preScriptsJson,omitempty"`
	PostScriptsJSON      string        `json:"postScriptsJson,omitempty"`
	HTTPSecretReference  string        `json:"httpSecretReference,omitempty"`
	HTTPSecretHeader     string        `json:"httpSecretHeader,omitempty"`
	FollowRedirects      bool          `json:"followRedirects,omitempty"`
	AMQP                 AMQPSettings  `json:"amqp,omitzero"`
	SQL                  bool          `json:"sql,omitempty"`
	Kafka                KafkaSettings `json:"kafka,omitzero"`
	SQLSettings          SQLSettings   `json:"sqlSettings,omitzero"`
	SQLRowLimit          string        `json:"sqlRowLimit,omitempty"`
	SQLTimeoutMS         string        `json:"sqlTimeoutMs,omitempty"`
	BrokerTimeoutMS      string        `json:"brokerTimeoutMs,omitempty"`
	MQTT                 MQTTSettings  `json:"mqtt,omitzero"`
	RPC                  RPCSettings   `json:"rpc,omitzero"`
	GRPC                 GRPCSettings  `json:"grpc,omitzero"`
	Redis                RedisSettings `json:"redis,omitzero"`
	SocketNetwork        string        `json:"socketNetwork,omitempty"`
	SocketSendCount      string        `json:"socketSendCount,omitempty"`
	SocketInterval       string        `json:"socketIntervalMs,omitempty"`
	SocketTimeoutMS      string        `json:"socketTimeoutMs,omitempty"`
	SocketFraming        string        `json:"socketFraming,omitempty"`
	SocketTLS            bool          `json:"socketTLS,omitempty"`
	SocketServerName     string        `json:"socketServerName,omitempty"`
	SocketCA             string        `json:"socketCA,omitempty"`
	Name                 string        `json:"name"`
	Method               string        `json:"method"`
	Target               string        `json:"target"`
	Headers              string        `json:"headers"`
	Body                 string        `json:"body"`
	HTTPBodyMode         string        `json:"httpBodyMode,omitempty"`
	HTTPTimeoutMS        string        `json:"httpTimeoutMs,omitempty"`
	ResponseBodyContains string        `json:"responseBodyContains,omitempty"`
	ExpectedHTTPStatus   string        `json:"expectedHttpStatus,omitempty"`
	HTTPBodySource       string        `json:"httpBodySource,omitempty"`
	MultipartJSON        string        `json:"multipartJson,omitempty"`
	GraphQL              bool          `json:"graphql,omitempty"`
	GraphQLVariables     string        `json:"graphqlVariables,omitempty"`
	GraphQLOperation     string        `json:"graphqlOperation,omitempty"`
	GraphQLSubscription  bool          `json:"graphqlSubscription,omitempty"`
	SSE                  bool          `json:"sse,omitempty"`
	SSEReconnects        string        `json:"sseReconnects,omitempty"`
	SSEReconnectDelay    string        `json:"sseReconnectDelayMs,omitempty"`
	WebSocket            bool          `json:"websocket,omitempty"`
	WebSocketProtocols   string        `json:"websocketProtocols,omitempty"`
	WebSocketAutomation  string        `json:"websocketAutomation,omitempty"`
}

// Workspace is local working state. Portable project files have their own
// format so they remain interchangeable with the existing application.
type Workspace struct {
	HistorySnapshotsEnabled bool              `json:"historySnapshotsEnabled,omitempty"`
	HistorySnapshots        []HistorySnapshot `json:"historySnapshots,omitempty"`
	DraftVariables          map[string]string `json:"draftVariables,omitempty"`
	Version                 int               `json:"version"`
	Project                 *Project          `json:"project,omitempty"`
	Draft                   Draft             `json:"draft"`
	Selected                int               `json:"selected,omitempty"`
	Page                    int               `json:"page,omitempty"`
	Modified                bool              `json:"modified,omitempty"`
	Variables               VariableScopes    `json:"variables,omitzero"`
}

func EncodeWorkspace(workspace Workspace) ([]byte, error) {
	if workspace.Variables.containsSensitiveData() || sensitiveVariables(workspace.DraftVariables) {
		return nil, errors.New("autosave paused for possible credentials")
	}
	if err := validateHistorySnapshots(workspace.HistorySnapshots); err != nil {
		return nil, err
	}
	if workspace.Project != nil {
		if err := validateEmptyFolders(workspace.Project.EmptyFolders); err != nil {
			return nil, err
		}
		for _, request := range workspace.Project.Requests {
			if request.containsSensitiveData() {
				return nil, errors.New("autosave paused for possible credentials")
			}
		}
	}
	draft := workspace.Draft
	if len(draft.PreScriptsJSON)+len(draft.PostScriptsJSON) > 256<<10 ||
		sensitiveBody.MatchString(draft.PreScriptsJSON) || sensitiveBody.MatchString(draft.PostScriptsJSON) ||
		hasPrivateKey(draft.PreScriptsJSON) || hasPrivateKey(draft.PostScriptsJSON) {
		return nil, errors.New("autosave paused for oversized scripts or possible script credentials")
	}
	if sensitiveBodySource(draft.HTTPBodySource) {
		return nil, errors.New("autosave paused for possible HTTP body source credentials")
	}
	if draft.MultipartJSON != "" {
		parts, err := ParseHTTPMultipart(draft.MultipartJSON)
		if err != nil {
			return nil, errors.New("autosave paused until the multipart JSON is valid")
		}
		if sensitiveMultipart(parts) || sensitiveBody.MatchString(draft.MultipartJSON) || hasPrivateKey(draft.MultipartJSON) {
			return nil, errors.New("autosave paused for possible multipart credentials")
		}
	}
	if draft.Kafka.Mode != "" && draft.Kafka.containsSensitiveData(draft.Target) {
		return nil, errors.New("autosave paused for possible Kafka credentials")
	}
	if draft.SQL && draft.SQLSettings.containsSensitiveData(draft.Target, draft.Body) {
		return nil, errors.New("autosave paused for possible SQL credentials")
	}
	if draft.AMQP.Mode != "" && (!draft.AMQP.Valid() || draft.AMQP.containsSensitiveData(draft.Target)) {
		return nil, errors.New("autosave paused for invalid AMQP settings or possible credentials")
	}
	if draft.MQTT.Mode != "" && (!draft.MQTT.Valid() || draft.MQTT.containsSensitiveData(draft.Target)) {
		return nil, errors.New("autosave paused for invalid MQTT settings or possible credentials")
	}
	if draft.Redis.Protocol != 0 && (strings.Contains(draft.Target, "@") || hasPrivateKey(draft.Redis.CAPEM) ||
		draft.Body != "" && draft.Redis.CheckPersistence(draft.Target, draft.Body) != nil) {
		return nil, errors.New("autosave paused for invalid Redis commands or possible credentials")
	}
	if (Request{URL: draft.Target, Body: draft.Body, BodyEncoding: draft.HTTPBodyMode, ResponseBodyContains: draft.ResponseBodyContains}).containsSensitiveData() || sensitiveBody.MatchString(draft.GraphQLVariables) ||
		sensitiveBody.MatchString(draft.WebSocketAutomation) || hasPrivateKey(draft.WebSocketAutomation) || hasPrivateKey(draft.SocketCA) {
		return nil, errors.New("autosave paused for possible credentials")
	}
	for line := range strings.SplitSeq(draft.Headers, "\n") {
		name, _, _ := strings.Cut(line, ":")
		if sensitiveKey.MatchString(strings.TrimSpace(name)) {
			return nil, errors.New("autosave paused for possible credentials")
		}
	}
	workspace.Version = 1
	data, err := json.Marshal(workspace)
	if err != nil {
		return nil, errors.New("could not encode the workspace")
	}
	if len(data) > MaxBytes {
		return nil, errors.New("workspace exceeds 8 MiB")
	}
	return data, nil
}

func DecodeWorkspace(data []byte) (Workspace, error) {
	var value Workspace
	if len(data) > MaxBytes || json.Unmarshal(data, &value) != nil || value.Version != 1 {
		return Workspace{}, errors.New("could not restore the saved workspace")
	}
	if err := validateHistorySnapshots(value.HistorySnapshots); err != nil {
		return Workspace{}, err
	}
	count := 0
	if value.Project != nil {
		if err := validateEmptyFolders(value.Project.EmptyFolders); err != nil {
			return Workspace{}, err
		}
		count = len(value.Project.Requests)
		for _, request := range value.Project.Requests {
			if err := request.ValidateScripts(); err != nil {
				return Workspace{}, err
			}
			if request.AMQP != nil && !request.AMQP.Valid() {
				return Workspace{}, errors.New("invalid saved AMQP settings")
			}
			if request.MQTT != nil && !request.MQTT.Valid() {
				return Workspace{}, errors.New("invalid saved MQTT settings")
			}
			if request.Redis != nil && !request.Redis.validProtocol() {
				return Workspace{}, errors.New("invalid saved Redis protocol")
			}
		}
	}
	if count > maxRequests || value.Selected < 0 || value.Selected > count || value.Page < 0 ||
		len(value.Variables.Environments) > MaxEnvironments {
		return Workspace{}, errors.New("could not restore the saved workspace")
	}
	if value.Draft.SocketNetwork != "" && value.Draft.SocketNetwork != "tcp" && value.Draft.SocketNetwork != "udp" {
		return Workspace{}, errors.New("invalid saved socket protocol")
	}
	if value.Draft.Redis.Protocol != 0 && !value.Draft.Redis.validProtocol() {
		return Workspace{}, errors.New("invalid saved Redis protocol")
	}
	if value.Draft.RPC.Protocol != "" && !value.Draft.RPC.Valid() {
		return Workspace{}, errors.New("invalid saved RPC protocol")
	}
	if value.Draft.MQTT.Mode != "" && !value.Draft.MQTT.Valid() {
		return Workspace{}, errors.New("invalid saved MQTT draft")
	}
	if value.Draft.AMQP.Mode != "" && !value.Draft.AMQP.Valid() {
		return Workspace{}, errors.New("invalid saved AMQP draft")
	}
	return value, nil
}

func validateHistorySnapshots(snapshots []HistorySnapshot) error {
	if len(snapshots) > MaxHistorySnapshots {
		return errors.New("too many saved history snapshots")
	}
	total := 0
	seen := make(map[string]bool, len(snapshots))
	for _, snapshot := range snapshots {
		total += len(snapshot.Request)
		if snapshot.ID == "" || seen[snapshot.ID] || total > MaxHistorySnapshotTotal {
			return errors.New("invalid saved history snapshot index")
		}
		seen[snapshot.ID] = true
		if _, err := OpenHistoryRequest(snapshot.Request); err != nil {
			return err
		}
	}
	return nil
}
