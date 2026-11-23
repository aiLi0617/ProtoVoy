package project

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"protovoy.dev/protovoy/internal/execution"
)

// readLegacyRequest converts an old request envelope to the Viva collection
// model only when its execution settings can be represented there.
func readLegacyRequest(data []byte) (Project, error) {
	var envelope struct {
		ID, Name, Target, ProtocolID, EnvironmentRef, Proxy, CreatedAt string
		TimeoutMS                                                      int
		Variables                                                      map[string]string
		Metadata                                                       map[string]json.RawMessage
		Payload                                                        json.RawMessage
		AuthRef                                                        *json.RawMessage
		PreScripts, PostScripts                                        []string
		Assertions                                                     []json.RawMessage
		RetryPolicy                                                    struct {
			MaxRetries int `json:"max_retries"`
			Backoff    int `json:"backoff_ms"`
		}
		TLS *struct {
			Verify            bool
			ClientCertificate string `json:"client_cert_ref"`
		}
	}
	if !decodeLegacy(data, &envelope) || strings.TrimSpace(envelope.Target) == "" {
		return Project{}, errors.New("invalid legacy request envelope")
	}
	if envelope.AuthRef != nil || envelope.Proxy != "" || envelope.RetryPolicy.MaxRetries != 0 ||
		!execution.ValidRequestTimeout(envelope.TimeoutMS) ||
		envelope.EnvironmentRef != "" && envelope.EnvironmentRef != "default-env" ||
		envelope.TLS != nil && (!envelope.TLS.Verify || envelope.TLS.ClientCertificate != "") ||
		len(envelope.Assertions) != 0 || !legacyMetadataSupported(envelope.ProtocolID, envelope.Metadata) {
		return Project{}, errors.New("legacy request has execution settings the Viva workspace cannot preserve")
	}
	request := Request{Name: envelope.Name, URL: envelope.Target, Variables: envelope.Variables,
		PreScripts: envelope.PreScripts, PostScripts: envelope.PostScripts}
	if request.HasScripts() && envelope.ProtocolID != "http" {
		return Project{}, errors.New("request scripts are supported for HTTP only")
	}
	var err error
	switch envelope.ProtocolID {
	case "graphql":
		return readGraphQLRequest(request, envelope.TimeoutMS, envelope.Payload)
	case "http":
		err = readLegacyHTTP(envelope.Payload, &request)
		if envelope.TimeoutMS != execution.DefaultRequestTimeoutMS {
			request.HTTPTimeoutMS = envelope.TimeoutMS
		}
	case "grpc":
		err = readLegacyGRPC(envelope.Payload, &request)
	case "tcp", "udp":
		err = readLegacySocket(envelope.ProtocolID, envelope.Payload, envelope.Metadata, &request)
	case "sse", "websocket":
		err = readLegacyStream(envelope.ProtocolID, envelope.Payload, &request)
	case "mqtt", "amqp", "redis":
		err = readLegacyBroker(envelope.ProtocolID, envelope.Payload, &request)
	case "sql":
		err = readLegacySQL(envelope.Payload, &request)
		if err == nil && envelope.TimeoutMS != execution.DefaultRequestTimeoutMS {
			request.SQL.TimeoutMS = envelope.TimeoutMS
		}
	case "jsonrpc", "soap":
		err = readLegacyRPC(envelope.ProtocolID, envelope.Payload, &request)
	case "kafka":
		err = readLegacyKafka(envelope.Payload, &request)
	default:
		return Project{}, errors.New("this legacy request protocol cannot be imported")
	}
	if err != nil {
		return Project{}, err
	}
	if envelope.TimeoutMS != 0 && envelope.TimeoutMS != execution.DefaultRequestTimeoutMS {
		switch {
		case request.isMessageBroker():
			request.BrokerTimeoutMS = envelope.TimeoutMS
		case request.Socket != nil:
			request.Socket.TimeoutMS = envelope.TimeoutMS
		case request.RPC != nil:
			request.RPC.TimeoutMS = envelope.TimeoutMS
		case request.GRPC != nil:
			request.GRPC.TimeoutMS = envelope.TimeoutMS
		case envelope.ProtocolID == "http" || envelope.ProtocolID == "sql":
		default:
			return Project{}, errors.New("legacy request uses an unsupported custom timeout")
		}
	}
	if err := validatePortableRequest(&request, true); err != nil {
		return Project{}, err
	}
	if request.Name == "" {
		request.Name = "Imported request"
	}
	return Project{Name: request.Name, Requests: []Request{request}}, nil
}

func readLegacyRPC(protocol string, data json.RawMessage, request *Request) error {
	var wrapper struct {
		Type  string
		Value json.RawMessage
	}
	if json.Unmarshal(data, &wrapper) != nil || wrapper.Type != "raw" {
		return errors.New("invalid legacy RPC payload")
	}
	if len(wrapper.Value) != 0 {
		if !decodeLegacy(data, &wrapper) || !isJSONObject(wrapper.Value) {
			return errors.New("invalid legacy RPC payload")
		}
		data = wrapper.Value
	}
	settings := RPCSettings{Protocol: protocol}
	var headers [][2]string
	if protocol == "jsonrpc" {
		var input struct {
			Type, Method string
			Params, ID   json.RawMessage
			Headers      [][2]string
		}
		if !decodeLegacy(data, &input) || strings.TrimSpace(input.Method) == "" {
			return errors.New("invalid legacy JSON-RPC payload")
		}
		settings.Method, settings.ID = input.Method, string(input.ID)
		request.Body, headers = string(input.Params), input.Headers
	} else {
		var input struct {
			Type, Version, Action, Envelope string
			Headers                         [][2]string
		}
		if !decodeLegacy(data, &input) {
			return errors.New("invalid legacy SOAP payload")
		}
		if _, err := execution.SOAPPayload(input.Version, input.Action, input.Envelope, input.Headers); err != nil {
			return err
		}
		settings.Version, settings.Action = input.Version, input.Action
		request.Body, headers = input.Envelope, input.Headers
	}
	var err error
	request.RPC = &settings
	request.Headers, err = uniqueHeaderMap(headers)
	return err
}

func readLegacyKafka(data json.RawMessage, request *Request) error {
	var envelope struct {
		Type  string
		Value json.RawMessage
	}
	if !decodeLegacy(data, &envelope) || envelope.Type != "raw" || !isJSONObject(envelope.Value) {
		return errors.New("invalid legacy Kafka payload")
	}
	payload := struct {
		execution.KafkaOperation
		SecurityProtocol, SASLMechanism, Username string
		execution.KafkaTLSReferences
		PasswordRef, KeyPasswordRef string
	}{
		KafkaOperation: execution.KafkaOperation{Mode: "produce", Encoding: "text", GroupID: "protovoy-consumer",
			OffsetReset: "latest", ReceiveLimit: 1},
		SecurityProtocol: "PLAINTEXT", SASLMechanism: "PLAIN",
	}
	if !decodeLegacy(envelope.Value, &payload) {
		return errors.New("invalid legacy Kafka settings")
	}
	if payload.KeyPasswordRef != "" {
		return errors.New("legacy Kafka private key password references require manual migration")
	}
	switch payload.SecurityProtocol {
	case "", "PLAINTEXT", "SSL", "SASL_SSL":
	default:
		return errors.New("legacy Kafka security protocol is unsupported; use TLS for SASL")
	}
	settings := KafkaSettings{
		Mode: payload.Mode, Topic: payload.Topic, Key: payload.Key, Encoding: payload.Encoding, GroupID: payload.GroupID,
		OffsetReset: payload.OffsetReset, AutoCommit: payload.AutoCommit, ReceiveLimit: strconv.Itoa(max(1, min(10_000, payload.ReceiveLimit))),
		SecurityProtocol: payload.SecurityProtocol, SASLMechanism: payload.SASLMechanism, Username: payload.Username,
		PasswordRef:        payload.PasswordRef,
		KafkaTLSReferences: payload.KafkaTLSReferences,
	}
	if payload.Partition != nil && *payload.Partition >= 0 {
		settings.Partition = strconv.FormatInt(int64(*payload.Partition), 10)
	}
	request.Kafka, request.Body = &settings, payload.Payload
	if request.containsSensitiveData() {
		return errors.New("remove possible credentials from the legacy Kafka request")
	}
	return nil
}

func readLegacySQL(data json.RawMessage, request *Request) error {
	var payload struct {
		Type  string
		Value struct {
			execution.SQLRequest
			Username, Password, PasswordRef string
		}
	}
	if !decodeLegacy(data, &payload) || payload.Type != "raw" {
		return errors.New("invalid legacy SQL payload")
	}
	if payload.Value.Password != "" {
		return errors.New("remove embedded SQL passwords before importing")
	}
	settings := SQLSettings{Username: payload.Value.Username, PasswordRef: payload.Value.PasswordRef,
		Transactional: payload.Value.Transactional, RowLimit: 500}
	if payload.Value.RowLimit != nil {
		settings.RowLimit = max(1, min(*payload.Value.RowLimit, 10_000))
	}
	if payload.Value.Parameters != nil {
		parameters, err := json.Marshal(payload.Value.Parameters)
		if err != nil {
			return errors.New("invalid legacy SQL parameters")
		}
		settings.Parameters = string(parameters)
	}
	request.SQL, request.Body = &settings, payload.Value.Statement
	return nil
}

func legacyMetadataSupported(protocol string, metadata map[string]json.RawMessage) bool {
	for key, value := range metadata {
		if (protocol == "tcp" || protocol == "udp") && (key == "socketPayloadFormat" || key == "socketSourceData") {
			continue
		}
		if protocol != "http" {
			return false
		}
		switch key {
		case "__protovoyAssertionSchemaVersion":
			if string(bytes.TrimSpace(value)) != "2" {
				return false
			}
		case "__protovoyAssertionsEnabled":
			var enabled *bool
			if json.Unmarshal(value, &enabled) != nil || enabled == nil {
				return false
			}
		case "__protovoyDisabledGeneratedHeaders":
			var headers []string
			if json.Unmarshal(value, &headers) != nil || len(headers) != 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func readLegacyHTTP(data []byte, request *Request) error {
	var payload struct {
		Type, Method, Body, BodyEncoding, BodySource string
		Headers                                      json.RawMessage
		Multipart                                    []execution.MultipartPart
		FollowRedirects                              bool
	}
	if !decodeLegacy(data, &payload) || payload.Type != "http" || payload.Method == "" {
		return errors.New("invalid legacy HTTP payload")
	}
	var pairs [][2]string
	if len(payload.Headers) != 0 && json.Unmarshal(payload.Headers, &pairs) != nil {
		var headers map[string]string
		if json.Unmarshal(payload.Headers, &headers) != nil {
			return errors.New("invalid legacy HTTP headers")
		}
		for key, value := range headers {
			pairs = append(pairs, [2]string{key, value})
		}
	}
	headers, err := uniqueHeaderMap(pairs)
	if err != nil {
		return err
	}
	request.Headers, request.Method, request.Body = headers, payload.Method, payload.Body
	request.BodySource = payload.BodySource
	request.FollowRedirects = payload.FollowRedirects
	if len(payload.Multipart) != 0 {
		request.Multipart, request.Body = payload.Multipart, ""
		// The legacy driver supplied its own multipart Content-Type.
		for name := range request.Headers {
			if strings.EqualFold(name, "Content-Type") {
				delete(request.Headers, name)
			}
		}
	} else if payload.BodyEncoding != "text" {
		request.BodyEncoding = payload.BodyEncoding
	}
	return nil
}

func decodeLegacy(data []byte, value any) bool {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(value) == nil && decoder.Decode(new(any)) == io.EOF
}

func readLegacyGRPC(data []byte, request *Request) error {
	if strings.Contains(request.URL, "@") {
		return errors.New("remove credentials from the legacy gRPC URL before importing")
	}
	var payload struct {
		Type, Service, Method, Mode, MessageBase64, DescriptorSetBase64 string
		MessageJSON                                                     *string
		Metadata                                                        [][2]string
	}
	if !decodeLegacy(data, &payload) || payload.Type != "grpc" {
		return errors.New("invalid legacy gRPC payload")
	}
	headers, err := uniqueHeaderMap(payload.Metadata)
	if err != nil {
		return err
	}
	request.Headers = headers
	request.Body = payload.MessageBase64
	if payload.MessageJSON != nil {
		if payload.MessageBase64 != "" {
			return errors.New("legacy gRPC payload contains both JSON and Base64 messages")
		}
		request.Body = *payload.MessageJSON
	}
	request.GRPC = &GRPCSettings{Service: payload.Service, Method: payload.Method, Mode: payload.Mode,
		DescriptorSetBase64: payload.DescriptorSetBase64, UseJSON: payload.MessageJSON != nil}
	return nil
}

func readLegacyBroker(protocol string, data []byte, request *Request) error {
	var header struct {
		Type  string
		Value json.RawMessage
	}
	if json.Unmarshal(data, &header) != nil {
		return errors.New("invalid legacy broker payload")
	}
	if header.Type == "raw" {
		if !decodeLegacy(data, &header) || !isJSONObject(header.Value) {
			return errors.New("invalid raw broker payload")
		}
		data = header.Value
	} else if header.Type != protocol {
		return errors.New("legacy broker payload does not match its protocol")
	}
	if strings.Contains(request.URL, "@") {
		return errors.New("remove broker credentials from the legacy request URL before importing")
	}
	switch protocol {
	case "mqtt":
		payload := struct {
			MQTTSettings
			Type, Payload, Password, CAPEMRef string
			CleanSession                      *bool
		}{MQTTSettings: MQTTSettings{Mode: "publish", Encoding: "text", KeepAlive: 30, ReceiveLimit: 1}}
		if !decodeLegacy(data, &payload) {
			return errors.New("invalid legacy MQTT payload")
		}
		if payload.Password != "" || payload.CAPEMRef != "" || hasPrivateKey(payload.CAPEM) {
			return errors.New("legacy MQTT credentials or secret references require manual migration")
		}
		if payload.CleanSession != nil {
			payload.PersistentSession = !*payload.CleanSession
		}
		request.MQTT, request.Body = &payload.MQTTSettings, payload.Payload
	case "amqp":
		payload := struct {
			AMQPSettings
			Type, Payload, Password string
		}{AMQPSettings: AMQPSettings{Mode: "publish", ExchangeType: "direct", Encoding: "text",
			ContentType: "application/octet-stream", Declare: true, ReceiveLimit: 1}}
		if !decodeLegacy(data, &payload) {
			return errors.New("invalid legacy AMQP payload")
		}
		if payload.Password != "" || hasPrivateKey(payload.CAPEM) {
			return errors.New("legacy AMQP credentials or secret references require manual migration")
		}
		request.AMQP, request.Body = &payload.AMQPSettings, payload.Payload
	case "redis":
		payload := struct {
			RedisSettings
			Type, Password string
			Database       json.RawMessage `json:"database"`
			Commands       json.RawMessage
		}{RedisSettings: RedisSettings{Protocol: 2}}
		if !decodeLegacy(data, &payload) {
			return errors.New("invalid legacy Redis payload")
		}
		if payload.Password != "" {
			return errors.New("legacy Redis credentials or secret references require manual migration")
		}
		if len(payload.Database) > 0 && string(payload.Database) != "null" {
			if json.Unmarshal(payload.Database, &payload.RedisSettings.Database) != nil {
				payload.RedisSettings.Database = string(payload.Database)
			}
			if db := payload.RedisSettings.Database; db != "" {
				if _, err := strconv.ParseUint(db, 10, 64); err != nil {
					return errors.New("legacy Redis database must be a nonnegative integer")
				}
			}
		}
		commands, err := execution.ParseRedisCommands(string(payload.Commands))
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(commands)
		if err != nil {
			return fmt.Errorf("could not encode legacy Redis commands: %w", err)
		}
		if err := payload.CheckPersistence(request.URL, string(encoded)); err != nil {
			return err
		}
		request.Redis, request.Body = &payload.RedisSettings, string(encoded)
	}
	return nil
}
