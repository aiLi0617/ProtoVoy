package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
	"protovoy.dev/protovoy/internal/scripting"
	"protovoy.dev/protovoy/internal/secrets"
)

type requestEnvelope struct {
	ID          string            `json:"id"`
	Protocol    string            `json:"protocolId"`
	Name        string            `json:"name"`
	Target      string            `json:"target"`
	Environment string            `json:"environmentRef"`
	Auth        *json.RawMessage  `json:"authRef"`
	Timeout     int               `json:"timeoutMs"`
	Proxy       string            `json:"proxy"`
	Metadata    json.RawMessage   `json:"metadata"`
	Created     string            `json:"createdAt"`
	Variables   map[string]string `json:"variables"`
	PreScripts  []string          `json:"preScripts"`
	PostScripts []string          `json:"postScripts"`
	Assertions  []json.RawMessage `json:"assertions"`
	Retry       struct {
		MaxRetries int `json:"max_retries"`
		Backoff    int `json:"backoff_ms"`
	} `json:"retryPolicy"`
	TLS *struct {
		Verify     bool   `json:"verify"`
		ClientCert string `json:"client_cert_ref"`
	} `json:"tls"`
	Payload json.RawMessage `json:"payload"`
}

type httpEnvelopePayload struct {
	execution.HTTPPayload
	Literal        bool    `json:"literal,omitempty"`
	Type           string  `json:"type"`
	Source         string  `json:"bodySource"`
	LastEventID    *string `json:"lastEventId"`
	ReconnectMax   int     `json:"reconnectMax"`
	ReconnectDelay *int    `json:"reconnectDelayMs"`
}

func (envelope requestEnvelope) historySnapshotID() string {
	var metadata struct{ HistorySnapshotID string }
	if json.Unmarshal(envelope.Metadata, &metadata) == nil && validID(metadata.HistorySnapshotID) {
		return metadata.HistorySnapshotID
	}
	return ""
}

type executionPlan struct {
	timeout time.Duration
	execute func(*agentJob, context.Context, string, string) agentEvent
}

func (agent *agentService) prepare(envelope requestEnvelope) (executionPlan, error) {
	if envelope.Auth != nil && envelope.Protocol != "http" || envelope.Proxy != "" || envelope.Retry.MaxRetries != 0 ||
		(envelope.Environment != "" && envelope.Environment != "default-env") ||
		(envelope.TLS != nil && (!envelope.TLS.Verify || envelope.TLS.ClientCert != "")) ||
		len(envelope.Assertions) != 0 {
		return executionPlan{}, errors.New("request uses execution settings this Local Agent does not support")
	}
	scripts := project.Request{PreScripts: envelope.PreScripts, PostScripts: envelope.PostScripts}
	if scripts.HasScripts() {
		if envelope.Protocol != "http" || envelope.Auth != nil {
			return executionPlan{}, errors.New("Local Agent scripts require HTTP without credential bindings")
		}
		if err := scripts.ValidateScripts(); err != nil {
			return executionPlan{}, err
		}
	}
	if !validID(envelope.ID) || !execution.ValidRequestTimeout(envelope.Timeout) {
		return executionPlan{}, errors.New("request requires a UUID and a timeout up to five minutes")
	}
	timeout := execution.RequestTimeout(envelope.Timeout)
	switch envelope.Protocol {
	case "http", "sse":
		var payload httpEnvelopePayload
		if !decodeExecutionPayload(envelope.Payload, &payload) || payload.Type != envelope.Protocol || payload.EventStream {
			return executionPlan{}, errors.New("invalid HTTP or SSE execution payload")
		}
		request, err := envelope.prepareHTTP(payload, timeout, true)
		if err != nil {
			return executionPlan{}, err
		}
		if request.EventStream {
			delay := time.Second
			if payload.ReconnectDelay != nil {
				delay = time.Duration(*payload.ReconnectDelay) * time.Millisecond
			}
			return executionPlan{execute: func(job *agentJob, ctx context.Context, id, requestID string) agentEvent {
				return job.executeSSE(ctx, id, requestID, request, payload.ReconnectMax, delay)
			}}, nil
		}
		return executionPlan{timeout: timeout, execute: func(job *agentJob, ctx context.Context, id, requestID string) agentEvent {
			authenticated := request
			if envelope.Auth != nil {
				var binding struct {
					Type      string `json:"type"`
					Reference string `json:"secretRef"`
					Header    string `json:"header"`
				}
				if !decodeExecutionPayload(*envelope.Auth, &binding) || binding.Type != "header" {
					return terminalFailure(ctx, "HTTP credential binding")
				}
				var err error
				authenticated.Headers, err = secrets.HTTPHeadersContext(ctx, authenticated.Headers, authenticated.Target, binding.Reference, binding.Header, authenticated.FollowRedirects)
				if err != nil {
					return terminalFailure(ctx, "HTTP credential binding")
				}
			}
			if scripts.HasScripts() {
				input, data, variables, err := scripting.PrepareHTTP(ctx, request, envelope.PreScripts, envelope.Variables)
				if err != nil {
					return terminalFailure(ctx, "pre-request script")
				}
				return job.executeHTTP(ctx, id, requestID, "http", input, func(response *execution.HTTPResponse, started time.Time) (*execution.HTTPResponse, error) {
					if len(envelope.PostScripts) == 0 {
						return response, nil
					}
					defer response.Body.Close()
					body, err := scripting.ReadHTTPPost(ctx, response, envelope.PostScripts, data, variables, started)
					if err != nil {
						return nil, err
					}
					return &execution.HTTPResponse{StatusCode: response.StatusCode, Headers: response.Headers,
						Body: io.NopCloser(bytes.NewReader(body))}, nil
				})
			}
			return job.execute(ctx, id, requestID, "http", authenticated)
		}}, nil
	case "jsonrpc", "soap":
		request, err := envelope.prepareRPC(timeout)
		if err != nil {
			return executionPlan{}, err
		}
		return executionPlan{timeout: timeout, execute: func(job *agentJob, ctx context.Context, id, requestID string) agentEvent {
			return job.execute(ctx, id, requestID, envelope.Protocol, request)
		}}, nil
	case "tcp", "udp":
		request, err := envelope.prepareSocket(timeout)
		if err != nil {
			return executionPlan{}, err
		}
		return executionPlan{timeout: timeout, execute: func(job *agentJob, ctx context.Context, id, requestID string) agentEvent {
			return job.executeSocket(ctx, id, requestID, request)
		}}, nil
	case "redis":
		payload, options, err := envelope.prepareRedis(agent.redisPasswordKeychain)
		if err != nil {
			return executionPlan{}, err
		}
		return executionPlan{timeout: timeout, execute: func(job *agentJob, ctx context.Context, id, requestID string) agentEvent {
			return job.executeRedis(ctx, id, requestID, payload, options, timeout)
		}}, nil
	case "mqtt":
		request, err := envelope.prepareMQTT(agent.mqttPasswordKeychain)
		if err != nil {
			return executionPlan{}, err
		}
		return executionPlan{timeout: timeout, execute: func(job *agentJob, ctx context.Context, id, requestID string) agentEvent {
			return job.executeMQTT(ctx, id, requestID, request)
		}}, nil
	case "amqp":
		request, err := envelope.prepareAMQP(agent.amqpPasswordKeychain)
		if err != nil {
			return executionPlan{}, err
		}
		return executionPlan{timeout: timeout, execute: func(job *agentJob, ctx context.Context, id, requestID string) agentEvent {
			return job.executeJSONMessages(ctx, id, requestID, "amqp", "AMQP channel open", request.Execute)
		}}, nil
	case "kafka":
		request, err := envelope.prepareKafka(agent.kafkaPasswordKeychain)
		if err != nil {
			return executionPlan{}, err
		}
		return executionPlan{timeout: timeout, execute: func(job *agentJob, ctx context.Context, id, requestID string) agentEvent {
			return job.executeJSONMessages(ctx, id, requestID, "kafka", "Kafka broker connected", request.Execute)
		}}, nil
	case "sql":
		request, err := agent.prepareSQL(envelope)
		if err != nil {
			return executionPlan{}, err
		}
		return executionPlan{timeout: timeout, execute: func(job *agentJob, ctx context.Context, id, requestID string) agentEvent {
			return job.executeJSONMessages(ctx, id, requestID, "sql", "SQL connection open", request.Execute)
		}}, nil
	case "grpc":
		request, err := envelope.prepareGRPC(timeout)
		if err != nil {
			return executionPlan{}, err
		}
		return executionPlan{timeout: timeout, execute: func(job *agentJob, ctx context.Context, id, requestID string) agentEvent {
			return job.executeGRPC(ctx, id, requestID, request)
		}}, nil
	default:
		return executionPlan{}, errors.New("unsupported execution protocol")
	}
}

func decodeExecutionPayload(data json.RawMessage, payload any) bool {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(payload) == nil && decoder.Decode(new(any)) == io.EOF
}

func (envelope requestEnvelope) prepareHTTP(input httpEnvelopePayload, timeout time.Duration, deferredCredentials ...bool) (execution.HTTPRequest, error) {
	if input.Source != "" && (envelope.Protocol != "http" || len(input.Source) > 1<<20 || !json.Valid([]byte(input.Source))) {
		return execution.HTTPRequest{}, errors.New("invalid HTTP body source")
	}
	payload := input.HTTPPayload
	if payload.TimeoutMS != 0 {
		return execution.HTTPRequest{}, errors.New("set timeoutMs on the execution envelope")
	}
	var target string
	var err error
	if envelope.Protocol == "sse" {
		if (payload.Method != "" && payload.Method != http.MethodGet) || payload.Body != "" || payload.BodyEncoding != "" ||
			len(payload.Multipart) != 0 || payload.FollowRedirects || input.Literal ||
			input.ReconnectMax < 0 || input.ReconnectMax > 10 ||
			(input.ReconnectDelay != nil && (*input.ReconnectDelay < 0 || *input.ReconnectDelay > 60_000)) {
			return execution.HTTPRequest{}, errors.New("invalid SSE request or reconnect settings")
		}
		if input.LastEventID != nil {
			payload.Headers = execution.SSEHeadersWithEventID(payload.Headers, *input.LastEventID)
		}
		target, payload, err = execution.PrepareSSE(envelope.Target, payload.Headers, envelope.Variables)
	} else {
		if input.LastEventID != nil || input.ReconnectMax != 0 || input.ReconnectDelay != nil {
			return execution.HTTPRequest{}, errors.New("reconnect settings require an SSE request")
		}
		target = envelope.Target
		if !input.Literal {
			target, payload, err = payload.ResolveVariables(target, envelope.Variables)
		}
	}
	if err != nil {
		return execution.HTTPRequest{}, err
	}
	if envelope.Auth != nil {
		var binding struct {
			Type      string `json:"type"`
			Reference string `json:"secretRef"`
			Header    string `json:"header"`
		}
		if !decodeExecutionPayload(*envelope.Auth, &binding) || binding.Type != "header" {
			return execution.HTTPRequest{}, errors.New("HTTP credential requires a header reference")
		}
		if len(deferredCredentials) == 1 && deferredCredentials[0] {
			if payload.FollowRedirects {
				return execution.HTTPRequest{}, errors.New("HTTP credential bindings require redirects disabled")
			}
			for _, pair := range payload.Headers {
				if strings.EqualFold(pair[0], binding.Header) {
					return execution.HTTPRequest{}, errors.New("credential header is already present in the request")
				}
			}
			err = secrets.ValidateHTTPBinding(binding.Reference, target, binding.Header)
		} else {
			payload.Headers, err = secrets.HTTPHeaders(payload.Headers, target, binding.Reference, binding.Header, payload.FollowRedirects)
		}
		if err != nil {
			return execution.HTTPRequest{}, err
		}
	}
	return prepareResolvedHTTP(target, payload, payload.FollowRedirects, timeout)
}

func prepareResolvedHTTP(target string, payload execution.HTTPPayload, redirects bool, timeout time.Duration) (execution.HTTPRequest, error) {
	probe, err := http.NewRequest(payload.Method, target, nil)
	if err != nil || payload.Method == "" || probe.URL.Hostname() == "" || probe.URL.User != nil ||
		(probe.URL.Scheme != "http" && probe.URL.Scheme != "https") {
		return execution.HTTPRequest{}, errors.New("enter a valid HTTP method and URL")
	}
	if len(payload.Headers) > 128 || len(payload.Body) > 1<<20 {
		return execution.HTTPRequest{}, errors.New("resolved request exceeds its header or body limit")
	}
	for _, header := range payload.Headers {
		if header[0] == "" || strings.ContainsFunc(header[0], func(c rune) bool {
			return !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", c))
		}) || strings.ContainsFunc(header[1], func(c rune) bool { return c < 32 && c != '\t' || c == 127 }) {
			return execution.HTTPRequest{}, errors.New("invalid HTTP header")
		}
	}
	payload, err = payload.EncodeBody()
	if err != nil {
		return execution.HTTPRequest{}, err
	}
	request := payload.Request(target)
	request.Timeout = timeout
	request.FollowRedirects = redirects
	return request, nil
}

func validID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, c := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}
