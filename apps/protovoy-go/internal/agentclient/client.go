package agentclient

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"protovoy.dev/protovoy/internal/execution"
)

const DefaultAddress = "127.0.0.1:39217"

type DoRequest func(*http.Request) (*http.Response, error)

type Client struct {
	workspaceVersion string
	mockVersion      string
	secretsVersion   string
	pluginsVersion   string
	pluginUpdates    bool
	origin           string
	token            string
	do               DoRequest
}

type Response struct {
	Status       int
	Headers      [][2]string
	Preview      string
	PreviewBytes int
	RawPreview   []byte
	Truncated    bool
}

func Pair(ctx context.Context, address, token string, do DoRequest) (*Client, error) {
	if address == "" {
		address = DefaultAddress
	}
	endpoint, err := netip.ParseAddrPort(address)
	if err != nil || endpoint.Port() == 0 || !endpoint.Addr().IsLoopback() || endpoint.Addr().Zone() != "" {
		return nil, errors.New("enter a numeric loopback Agent address such as " + DefaultAddress)
	}
	if token == "" || len(token) > 1024 || strings.ContainsFunc(token, func(c rune) bool { return c < 33 || c > 126 }) {
		return nil, errors.New("enter the Local Agent pairing token")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	client := &Client{origin: "http://" + endpoint.String(), do: do}
	var health struct {
		Service             string
		ProtocolAPIVersion  string
		AuthRequired        bool
		WorkspaceAPIVersion string
		MockAPIVersion      string
		SecretsAPIVersion   string
		PluginsAPIVersion   string
		PluginUpdates       bool
	}
	if err := client.json(ctx, http.MethodGet, "/health", nil, &health); err != nil {
		return nil, err
	}
	if health.Service != "protovoy-agent" || health.ProtocolAPIVersion != "1" || !health.AuthRequired {
		return nil, errors.New("Local Agent identity or protocol version does not match")
	}
	client.token = token
	client.workspaceVersion = health.WorkspaceAPIVersion
	client.mockVersion = health.MockAPIVersion
	client.secretsVersion = health.SecretsAPIVersion
	client.pluginsVersion = health.PluginsAPIVersion
	client.pluginUpdates = health.PluginUpdates
	var session struct{ Token string }
	err = client.json(ctx, http.MethodPost, "/v1/session", nil, &session)
	client.token = session.Token
	if err != nil {
		return nil, err
	}
	if session.Token == "" || len(session.Token) > 1024 || strings.ContainsFunc(session.Token, func(c rune) bool { return c < 33 || c > 126 }) {
		return nil, errors.New("Local Agent returned an invalid session")
	}
	return client, nil
}

func (client *Client) RunHTTP(ctx context.Context, target string, payload execution.HTTPPayload, publish func(Response)) (Response, error) {
	return client.runHTTPExecution(ctx, target, "http", map[string]any{
		"type": "http", "literal": true, "method": payload.Method, "headers": append([][2]string{}, payload.Headers...),
		"body": payload.Body, "followRedirects": payload.FollowRedirects,
		"bodyEncoding": payload.BodyEncoding, "multipart": payload.Multipart,
	}, nil, payload.TimeoutMS, publish)
}

func (client *Client) RunRPC(ctx context.Context, target, protocol string, input map[string]any, variables map[string]string, timeoutMS int, publish func(Response)) (Response, error) {
	return client.runHTTPExecution(ctx, target, protocol, map[string]any{"type": "raw", "value": input}, variables, timeoutMS, publish)
}

func (client *Client) runHTTPExecution(ctx context.Context, target, protocol string, payload map[string]any, variables map[string]string, timeoutMS int, publish func(Response)) (Response, error) {
	if !execution.ValidRequestTimeout(timeoutMS) {
		return Response{}, errors.New("HTTP timeout must be at most five minutes")
	}
	if timeoutMS == 0 {
		timeoutMS = execution.DefaultRequestTimeoutMS
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond+5*time.Second)
	defer cancel()
	stream, err := client.openExecutionWithVariables(ctx, target, protocol, payload, variables, timeoutMS)
	if err != nil {
		return Response{}, err
	}
	defer stream.Close()
	result, finished, err := readEvents(stream.body, publish)
	stream.finished = finished
	return result, err
}

func (client *Client) openExecution(ctx context.Context, target, protocol string, payload map[string]any) (*executionStream, error) {
	return client.openExecutionWithVariables(ctx, target, protocol, payload, nil, execution.DefaultRequestTimeoutMS)
}

func (client *Client) openExecutionWithVariables(ctx context.Context, target, protocol string, payload map[string]any, variables map[string]string, timeoutMS int) (*executionStream, error) {
	if target == "" {
		return nil, errors.New("enter a request URL for Local Agent execution")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	requestID := savedRequestID(ctx)
	if requestID == "" {
		requestID = NewRequestID()
	}
	// The start may have reached the Agent even if the user just cancelled.
	// Give the acknowledgment a short deadline so cleanup can use its ID.
	startCtx, stopStart := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	var started struct{ ExecutionID string }
	err := client.json(startCtx, http.MethodPost, "/v1/executions", map[string]any{
		"id": requestID, "protocolId": protocol, "name": "Viva " + protocol + " request", "target": target,
		"timeoutMs": timeoutMS, "retryPolicy": map[string]int{"max_retries": 0, "backoff_ms": 0},
		"tls": map[string]bool{"verify": true}, "metadata": executionMetadata(ctx),
		"preScripts": httpScriptsFromContext(ctx).pre, "postScripts": httpScriptsFromContext(ctx).post, "createdAt": time.Now().UTC(),
		"payload": payload, "variables": variables,
		"authRef": ctx.Value(httpAuthKey{}),
	}, &started)
	stopStart()
	if err != nil {
		return nil, err
	}
	if !validExecutionID(started.ExecutionID) {
		return nil, errors.New("Local Agent returned an invalid execution ID")
	}
	path := "/v1/executions/" + started.ExecutionID
	stream := &executionStream{cancel: func() {
		cancelCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		_ = client.json(cancelCtx, http.MethodPost, path+"/cancel", nil, nil)
	}}
	response, err := client.request(ctx, http.MethodGet, path+"/events", "text/event-stream", nil)
	if err != nil {
		stream.Close()
		return nil, err
	}
	stream.body = response.Body
	if !execution.IsEventStream(response.StatusCode, response.Header.Get("Content-Type")) {
		stream.Close()
		return nil, errors.New("Local Agent returned an invalid event stream")
	}
	return stream, nil
}

type requestIDKey struct{}
type snapshotIDKey struct{}

func WithHistorySnapshotID(ctx context.Context, id string) context.Context {
	if !validExecutionID(id) {
		return ctx
	}
	return context.WithValue(ctx, snapshotIDKey{}, id)
}

func historySnapshotID(ctx context.Context) string {
	id, _ := ctx.Value(snapshotIDKey{}).(string)
	return id
}

func executionMetadata(ctx context.Context) map[string]string {
	data := make(map[string]string)
	if id := historySnapshotID(ctx); id != "" {
		data["historySnapshotId"] = id
	}
	return data
}

func WithSavedRequestID(ctx context.Context, id string) context.Context {
	if !validExecutionID(id) {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey{}, id)
}

func savedRequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

func NewRequestID() string {
	var id [16]byte
	rand.Read(id[:])
	id[6], id[8] = id[6]&0x0f|0x40, id[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:])
}

type agentEvent struct {
	Type         string
	Code         string
	Status       int
	Headers      [][2]string
	ContentType  string
	DataBase64   string `json:"data_base64"`
	Preview      *string
	JSONPreview  string `json:"jsonPreview"`
	Done         bool
	Size         int
	MessageIndex int
	MessageEnd   bool
	GRPCStatus   *int
}

func nextAgentEvent(decoder *execution.SSEDecoder) (agentEvent, error) {
	for {
		frame, err := decoder.Next()
		if err != nil {
			return agentEvent{}, errors.New("Local Agent event stream ended before completion")
		}
		if frame.Type != "execution" && frame.Type != "message" {
			continue
		}
		var event agentEvent
		if json.Unmarshal([]byte(frame.Data), &event) != nil {
			return agentEvent{}, errors.New("Local Agent returned an invalid execution event")
		}
		return event, nil
	}
}

type executionStream struct {
	body     io.ReadCloser
	cancel   func()
	finished bool
}

func (stream *executionStream) Close() error {
	if stream.body != nil {
		stream.body.Close()
	}
	if !stream.finished {
		stream.finished = true
		stream.cancel()
	}
	return nil
}

func readEvents(body io.Reader, publish func(Response)) (Response, bool, error) {
	decoder := execution.NewSSEDecoder(body, "", 0)
	var result Response
	var preview []byte
	rawPreview := true
	for {
		event, err := nextAgentEvent(decoder)
		if err != nil {
			return result, false, err
		}
		switch event.Type {
		case "response_meta":
			result.Status = event.Status
			headers := make(http.Header)
			for _, pair := range event.Headers {
				headers.Add(pair[0], pair[1])
			}
			result.Headers = execution.PreviewResponseHeaders(headers)
			publish(result)
		case "response_chunk":
			var data []byte
			if event.DataBase64 != "" {
				data, err = base64.StdEncoding.DecodeString(event.DataBase64)
				if err != nil {
					return result, false, errors.New("Local Agent returned invalid response bytes")
				}
			} else if event.Done && event.Preview != nil && len(preview) == 0 {
				data = []byte(*event.Preview)
				rawPreview = false
			}
			if len(data) == 0 {
				continue
			}
			space := execution.PreviewLimit - len(preview)
			truncated := len(data) > space
			if space == 0 && result.Truncated {
				continue
			}
			result.Truncated = result.Truncated || truncated
			preview = append(preview, data[:min(space, len(data))]...)
			result.PreviewBytes = len(preview)
			result.Preview = strings.ToValidUTF8(string(preview), "\uFFFD")
			publish(result)
		case "completed":
			if result.Status < 100 || result.Status > 999 {
				return result, true, errors.New("Local Agent completed without HTTP response metadata")
			}
			if rawPreview {
				result.RawPreview = preview
			}
			return result, true, nil
		case "failed":
			if event.Code == "timeout" {
				return result, true, context.DeadlineExceeded
			}
			return result, true, errors.New("Local Agent HTTP execution failed")
		case "cancelled":
			return result, true, context.Canceled
		}
	}
}

func (client *Client) json(ctx context.Context, method, path string, input, output any) error {
	var body []byte
	if input != nil {
		var err error
		body, err = json.Marshal(input)
		if err != nil {
			return errors.New("could not encode Local Agent request")
		}
	}
	response, err := client.request(ctx, method, path, "application/json", body)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if output != nil && json.NewDecoder(io.LimitReader(response.Body, 16<<10)).Decode(output) != nil {
		return errors.New("Local Agent returned invalid JSON")
	}
	return nil
}

func (client *Client) request(ctx context.Context, method, path, accept string, body []byte) (*http.Response, error) {
	return client.requestWithHeaders(ctx, method, path, accept, body, nil)
}

func (client *Client) requestWithHeaders(ctx context.Context, method, path, accept string, body []byte, headers http.Header) (*http.Response, error) {
	response, err := client.requestRaw(ctx, method, path, accept, body, headers)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		response.Body.Close()
		switch response.StatusCode {
		case http.StatusNotFound:
			return nil, ErrNotFound
		case http.StatusPreconditionFailed:
			return nil, ErrConflict
		case http.StatusUnauthorized:
			return nil, errors.New("Local Agent rejected credentials. Check the token or pair again")
		case http.StatusForbidden:
			return nil, errors.New("Local Agent rejected this origin or address")
		default:
			return nil, fmt.Errorf("Local Agent rejected the request (HTTP %d)", response.StatusCode)
		}
	}
	return response, nil
}

func (client *Client) requestRaw(ctx context.Context, method, path, accept string, body []byte, headers http.Header) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, client.origin+path, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("could not prepare Local Agent request")
	}
	request.Header.Set("Accept", accept)
	for name, values := range headers {
		request.Header[name] = append([]string(nil), values...)
	}
	if client.token != "" {
		request.Header.Set("Authorization", "Bearer "+client.token)
		request.Header.Set("X-ProtoVoy-Protocol-Api-Version", "1")
	}
	if len(body) != 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("could not reach Local Agent")
	}
	return response, nil
}

func validExecutionID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, c := range id {
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
