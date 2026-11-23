package project

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"protovoy.dev/protovoy/internal/execution"
)

const (
	MaxBytes            = 8 << 20
	maxRequests         = 10_000
	maxCollectionDepth  = 64
	legacyProjectFormat = "protovoy-project"
	multiProjectFormat  = "protovoy-multiprotocol-project"
)

type Project struct {
	Name         string
	Requests     []Request
	EmptyFolders [][]string `json:"emptyFolders,omitempty"`
	// Collection variables use Workspace.Variables for local persistence.
	CollectionVariables map[string]string `json:"-"`
}

type projectFile struct {
	Format              string            `json:"format"`
	Version             *int              `json:"version"`
	Name                string            `json:"name"`
	ExportedAt          string            `json:"exportedAt,omitempty"`
	Requests            []*Request        `json:"requests"`
	EmptyFolders        [][]string        `json:"emptyFolders,omitempty"`
	CollectionVariables map[string]string `json:"collectionVariables,omitempty"`
}

type Request struct {
	PreScripts           []string                    `json:"preScripts,omitempty"`
	PostScripts          []string                    `json:"postScripts,omitempty"`
	HTTPSecretReference  string                      `json:"httpSecretReference,omitempty"`
	HTTPSecretHeader     string                      `json:"httpSecretHeader,omitempty"`
	LocalID              string                      `json:"localId,omitempty"`
	FollowRedirects      bool                        `json:"followRedirects,omitempty"`
	AMQP                 *AMQPSettings               `json:"amqp,omitempty"`
	SQL                  *SQLSettings                `json:"sql,omitempty"`
	Kafka                *KafkaSettings              `json:"kafka,omitempty"`
	MQTT                 *MQTTSettings               `json:"mqtt,omitempty"`
	RPC                  *RPCSettings                `json:"rpc,omitempty"`
	GRPC                 *GRPCSettings               `json:"grpc,omitempty"`
	Redis                *RedisSettings              `json:"redis,omitempty"`
	Socket               *execution.SocketPayload    `json:"socket,omitempty"`
	Name                 string                      `json:"name"`
	Method               string                      `json:"method"`
	URL                  string                      `json:"url"`
	Headers              map[string]string           `json:"headers"`
	Body                 string                      `json:"body,omitempty"`
	BodyEncoding         string                      `json:"bodyEncoding,omitempty"`
	HTTPTimeoutMS        int                         `json:"httpTimeoutMs,omitempty"`
	ExpectedHTTPStatus   int                         `json:"expectedHttpStatus,omitempty"`
	ResponseBodyContains string                      `json:"responseBodyContains,omitempty"`
	BrokerTimeoutMS      int                         `json:"brokerTimeoutMs,omitempty"`
	BodySource           string                      `json:"bodySource,omitempty"`
	Multipart            []execution.MultipartPart   `json:"multipart,omitempty"`
	CollectionPath       []string                    `json:"collectionPath,omitempty"`
	Variables            map[string]string           `json:"variables,omitempty"`
	GraphQL              *execution.GraphQLDocument  `json:"graphql,omitempty"`
	SSE                  *execution.SSEOptions       `json:"sse,omitempty"`
	WebSocket            *execution.WebSocketOptions `json:"websocket,omitempty"`
}

type RPCSettings struct {
	TimeoutMS int    `json:"timeoutMs,omitempty"`
	Protocol  string `json:"protocol"`
	Method    string `json:"method,omitempty"`
	ID        string `json:"id,omitempty"`
	Version   string `json:"version,omitempty"`
	Action    string `json:"action,omitempty"`
}

func (settings RPCSettings) Valid() bool {
	return settings.Protocol == "jsonrpc" || settings.Protocol == "soap"
}

type GRPCSettings struct {
	TimeoutMS           int    `json:"timeoutMs,omitempty"`
	Service             string `json:"service"`
	Method              string `json:"method"`
	Mode                string `json:"mode"`
	UseJSON             bool   `json:"useJson,omitempty"`
	DescriptorSetBase64 string `json:"descriptorSetBase64,omitempty"`
}

func (settings GRPCSettings) ValidatePayload(body string) error {
	if !execution.ValidRequestTimeout(settings.TimeoutMS) {
		return errors.New("invalid gRPC timeout")
	}
	_, err := execution.DecodeGRPCDescriptor(settings.DescriptorSetBase64)
	if err != nil {
		return err
	}
	if settings.UseJSON && strings.TrimSpace(body) == "" {
		return errors.New("enter a gRPC JSON message")
	}
	return nil
}

func (value *Project) Append(request Request) error {
	if len(value.Requests) >= maxRequests {
		return errors.New("project exceeds 10,000 requests")
	}
	value.Requests = append(value.Requests, request)
	return nil
}

func Read(input io.Reader) (Project, error) {
	data, err := io.ReadAll(io.LimitReader(input, MaxBytes+1))
	if err != nil {
		return Project{}, errors.New("could not read the project file")
	}
	if len(data) > MaxBytes {
		return Project{}, errors.New("project file exceeds 8 MiB")
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))

	var metadata struct {
		Format        string
		ProtocolID    string
		SchemaVersion *int
		Log           *struct{ Version string }
		Info          *struct{ Schema string }
		OpenAPI       string
	}
	if json.Unmarshal(data, &metadata) != nil {
		if !json.Valid(data) {
			return readOpenAPIYAML(data)
		}
		return Project{}, errors.New("expected a ProtoVoy project or supported request file")
	}
	if metadata.Format == "" && metadata.SchemaVersion != nil {
		return readTeamSnapshot(data)
	}
	if metadata.Format == "" && metadata.ProtocolID != "" {
		return readLegacyRequest(data)
	}
	if metadata.Format == "" && metadata.Log != nil {
		return readHAR(data)
	}
	if metadata.Format == "" && metadata.OpenAPI != "" {
		return readOpenAPI(data)
	}
	if metadata.Format == "" && metadata.Info != nil {
		switch metadata.Info.Schema {
		case "https://schema.getpostman.com/json/collection/v2.1.0/collection.json",
			"https://schema.postman.com/json/collection/v2.1.0/collection.json":
			return readPostman(data)
		}
	}
	if metadata.Format != legacyProjectFormat && metadata.Format != multiProjectFormat {
		return Project{}, errors.New("expected a ProtoVoy project or supported request JSON file")
	}
	multi := metadata.Format == multiProjectFormat
	decoder := json.NewDecoder(bytes.NewReader(data))
	if multi {
		decoder.DisallowUnknownFields()
	}
	var document projectFile
	if decoder.Decode(&document) != nil || document.Requests == nil {
		return Project{}, errors.New("invalid ProtoVoy project file")
	}
	if (multi && (document.Version == nil || *document.Version != 2)) ||
		(!multi && document.Version != nil && *document.Version != 1) {
		return Project{}, errors.New("unsupported ProtoVoy project version")
	}
	if len(document.Requests) > maxRequests {
		return Project{}, errors.New("project exceeds 10,000 requests")
	}
	if !multi && len(document.EmptyFolders) != 0 {
		return Project{}, errors.New("empty folders require a v2 project")
	}
	name := document.Name
	if strings.TrimSpace(name) == "" {
		name = "ProtoVoy Import"
	}
	result := Project{Name: name, Requests: make([]Request, 0, len(document.Requests))}
	if multi {
		result.CollectionVariables = document.CollectionVariables
		result.EmptyFolders = document.EmptyFolders
	}
	if err := validateEmptyFolders(result.EmptyFolders); err != nil {
		return Project{}, err
	}
	for index, request := range document.Requests {
		if err := validatePortableRequest(request, multi); err != nil {
			return Project{}, fmt.Errorf("request %d: %w", index+1, err)
		}
		if request.Name == "" {
			request.Name = "Imported request"
		}
		request.LocalID = ""
		if request.Method == "" && request.protocolCount() == 0 {
			request.Method = http.MethodGet
		}
		result.Requests = append(result.Requests, *request)
	}
	return result, nil
}

func validateEmptyFolders(folders [][]string) error {
	if len(folders) > maxRequests {
		return errors.New("project exceeds 10,000 empty folders")
	}
	for _, path := range folders {
		if len(path) == 0 || len(path) > maxCollectionDepth+3 {
			return errors.New("invalid empty folder path")
		}
		for _, name := range path {
			if strings.TrimSpace(name) == "" {
				return errors.New("invalid empty folder name")
			}
		}
	}
	return nil
}

func (request Request) protocolCount() int {
	count := 0
	if request.RPC != nil {
		count++
	}
	for _, present := range []bool{request.GraphQL != nil, request.SSE != nil, request.WebSocket != nil, request.Socket != nil, request.Redis != nil, request.GRPC != nil, request.MQTT != nil, request.AMQP != nil, request.SQL != nil, request.Kafka != nil} {
		if present {
			count++
		}
	}
	return count
}

func (request Request) isMessageBroker() bool {
	return request.Redis != nil || request.MQTT != nil || request.AMQP != nil || request.Kafka != nil
}

const MaxResponseBodyAssertionBytes = 4 * 1024

func validatePortableRequest(request *Request, multi bool) error {
	if request == nil || (strings.TrimSpace(request.URL) == "" && (!multi || request.Redis == nil && request.MQTT == nil && request.AMQP == nil && request.SQL == nil && request.Kafka == nil)) {
		return errors.New("a target is required")
	}
	count := request.protocolCount()
	if err := request.ValidateScripts(); err != nil {
		return err
	}
	if request.HasScripts() && !multi {
		return errors.New("HTTP scripts require a v2 project")
	}
	if request.HTTPSecretReference != "" || request.HTTPSecretHeader != "" {
		protocol := request.CollectionHTTPProtocol()
		if !multi || (protocol != "http" && protocol != "graphql") || request.FollowRedirects ||
			request.HTTPSecretReference == "" || !execution.ValidPasswordReference(request.HTTPSecretReference) || request.HTTPSecretHeader == "" {
			return errors.New("HTTP credential bindings require a v2 HTTP or GraphQL request, a reference and header, and redirects disabled")
		}
	}
	if request.ResponseBodyContains != "" && (!multi || request.CollectionHTTPProtocol() == "" || len(request.ResponseBodyContains) > MaxResponseBodyAssertionBytes) {
		return errors.New("response body assertion requires a v2 HTTP-based request and a value within 4 KiB")
	}
	if status := request.ExpectedHTTPStatus; status != 0 &&
		(!multi || status < 200 || status > 599 || request.CollectionHTTPProtocol() == "") {
		return errors.New("expected HTTP status requires a v2 HTTP-based request and a status from 200 to 599")
	}
	if !multi && count != 0 {
		return errors.New("portable v1 projects support HTTP requests only")
	}
	if request.FollowRedirects && (!multi || count != 0) {
		return errors.New("HTTP redirect settings require a v2 HTTP request")
	}
	if request.HTTPTimeoutMS != 0 && (!multi || count != 0 || !execution.ValidRequestTimeout(request.HTTPTimeoutMS)) {
		return errors.New("custom timeout requires a v2 HTTP request and a value up to five minutes")
	}
	if request.BodyEncoding != "" || request.BodySource != "" || len(request.Multipart) != 0 {
		if !multi || count != 0 {
			return errors.New("HTTP body formats require a v2 HTTP request")
		}
		if err := request.ValidateHTTPBody(); err != nil {
			return err
		}
	}
	if count > 1 {
		return errors.New("conflicting protocol settings")
	}
	if request.BrokerTimeoutMS != 0 && (!multi || !execution.ValidRequestTimeout(request.BrokerTimeoutMS) || !request.isMessageBroker()) {
		return errors.New("custom broker timeout requires Redis, MQTT, AMQP, or Kafka and a value up to five minutes")
	}
	if request.RPC != nil && (!request.RPC.Valid() || !execution.ValidRequestTimeout(request.RPC.TimeoutMS)) {
		return errors.New("invalid RPC protocol")
	}
	if request.SSE != nil && !request.SSE.Valid() {
		return errors.New("invalid SSE reconnect settings")
	}
	if request.WebSocket != nil {
		if _, err := request.WebSocket.Frames(); err != nil {
			return err
		}
	}
	if request.MQTT != nil && (!request.MQTT.Valid() || request.MQTT.Topic == "") {
		return errors.New("invalid MQTT settings")
	}
	if request.AMQP != nil && (!request.AMQP.Valid() || !request.AMQP.HasDestination()) {
		return errors.New("invalid AMQP settings")
	}
	if request.SQL != nil {
		if err := request.SQL.Validate(request.URL, request.Body); err != nil {
			return err
		}
	}
	if request.Kafka != nil {
		if err := request.Kafka.Validate(request.URL, request.Body); err != nil {
			return err
		}
	}
	for key, value := range request.Headers {
		if !validHeader(key, value) {
			return errors.New("invalid header")
		}
	}
	if request.Redis != nil {
		if !request.Redis.validProtocol() {
			return errors.New("invalid Redis protocol")
		}
		if _, err := execution.ParseRedisCommands(request.Body); err != nil {
			return err
		}
	}
	if request.Socket != nil && (request.Socket.ExpectedTarget != "" ||
		!request.Socket.ValidSendOptions() ||
		!execution.ValidRequestTimeout(request.Socket.TimeoutMS) ||
		(request.Socket.Network != "tcp" && request.Socket.Network != "udp")) {
		return errors.New("invalid socket settings")
	}
	if request.GRPC != nil {
		if err := execution.ValidateGRPCSelection(request.GRPC.Service, request.GRPC.Method, request.GRPC.Mode); err != nil {
			return err
		}
		if err := request.GRPC.ValidatePayload(request.Body); err != nil {
			return err
		}
	}
	if request.GraphQL != nil {
		if err := ValidateGraphQLTimeout(*request.GraphQL); err != nil {
			return err
		}
	}
	return nil
}
func validHeader(key, value string) bool {
	return key != "" && strings.TrimSpace(key) == key && !strings.ContainsAny(key, ":\r\n") && !strings.ContainsAny(value, "\r\n")
}

func (request Request) HeaderText() string {
	names := make([]string, 0, len(request.Headers))
	for name := range request.Headers {
		names = append(names, name)
	}
	sort.Strings(names)
	var out strings.Builder
	for _, name := range names {
		if out.Len() > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(name)
		out.WriteString(": ")
		out.WriteString(request.Headers[name])
	}
	return out.String()
}
