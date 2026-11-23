//go:build !js

package scripting

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"protovoy.dev/protovoy/internal/execution"
)

type httpScriptRequest struct {
	ProtocolID string `json:"protocolId"`
	Target     string `json:"target"`
	Payload    struct {
		Type         string      `json:"type"`
		Method       string      `json:"method"`
		Headers      [][2]string `json:"headers"`
		Body         string      `json:"body"`
		BodyEncoding string      `json:"bodyEncoding,omitempty"`
		Follow       bool        `json:"followRedirects"`
	} `json:"payload"`
}

func PrepareHTTP(ctx context.Context, request execution.HTTPRequest, pre []string, variables map[string]string) (execution.HTTPRequest, json.RawMessage, map[string]string, error) {
	value := httpScriptRequest{ProtocolID: "http", Target: request.Target}
	value.Payload.Type, value.Payload.Method = "http", request.Method
	value.Payload.Headers = append([][2]string{}, request.Headers...)
	value.Payload.Body, value.Payload.Follow = request.Body, request.FollowRedirects
	if !utf8.ValidString(request.Body) {
		value.Payload.Body = base64.StdEncoding.EncodeToString([]byte(request.Body))
		value.Payload.BodyEncoding = "base64"
	}
	data, _ := json.Marshal(value)
	if len(pre) != 0 {
		result, err := Run(ctx, pre, data, variables, nil)
		if err != nil {
			return execution.HTTPRequest{}, nil, nil, fmt.Errorf("pre-request script: %w", err)
		}
		value = httpScriptRequest{}
		if json.Unmarshal(result.Request, &value) != nil || value.ProtocolID != "http" ||
			value.Payload.Type != "http" || value.Payload.Method == "" || value.Target == "" {
			return execution.HTTPRequest{}, nil, nil, errors.New("pre-request script produced an invalid HTTP request")
		}
		request.Target, request.Method = value.Target, value.Payload.Method
		request.Headers, request.Body, request.FollowRedirects = value.Payload.Headers, value.Payload.Body, value.Payload.Follow
		switch value.Payload.BodyEncoding {
		case "":
		case "base64":
			body, err := base64.StdEncoding.Strict().DecodeString(value.Payload.Body)
			if err != nil {
				return execution.HTTPRequest{}, nil, nil, errors.New("pre-request script produced an invalid Base64 body")
			}
			request.Body = string(body)
		default:
			return execution.HTTPRequest{}, nil, nil, errors.New("pre-request script produced an unsupported body encoding")
		}
		data, variables = result.Request, result.Variables
	}
	return request, data, variables, nil
}

func ExecuteHTTPPost(ctx context.Context, client execution.HTTPClient, request execution.HTTPRequest, post []string, data json.RawMessage, variables map[string]string) (int, []byte, error) {
	start := time.Now()
	response, err := client.Execute(ctx, request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	body, err := ReadHTTPPost(ctx, response, post, data, variables, start)
	return response.StatusCode, body, err
}

func ReadHTTPPost(ctx context.Context, response *execution.HTTPResponse, post []string, data json.RawMessage, variables map[string]string, start time.Time) ([]byte, error) {
	const limit = 1 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if len(body) > limit {
		return nil, errors.New("post-response script response exceeds 1 MiB")
	}
	headers := make(map[string]string, len(response.Headers))
	for name, values := range response.Headers {
		headers[strings.ToLower(name)] = strings.Join(values, ", ")
	}
	scriptBody, bodyEncoding := string(body), ""
	if !utf8.Valid(body) {
		scriptBody, bodyEncoding = base64.StdEncoding.EncodeToString(body), "base64"
	}
	input, _ := json.Marshal(struct {
		Status       int               `json:"status"`
		Headers      map[string]string `json:"headers"`
		Body         string            `json:"body"`
		BodyEncoding string            `json:"bodyEncoding,omitempty"`
		Duration     int64             `json:"durationMs"`
		Bytes        int               `json:"bytesReceived"`
	}{response.StatusCode, headers, scriptBody, bodyEncoding, time.Since(start).Milliseconds(), len(body)})
	if _, err := Run(ctx, post, data, variables, input); err != nil {
		return nil, fmt.Errorf("post-response script: %w", err)
	}
	return body, nil
}
