package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"syscall/js"

	"protovoy.dev/protovoy/internal/execution"
)

const localTarget = false

func execute(ctx context.Context, target string, payload execution.HTTPPayload) (*execution.HTTPResponse, error) {
	return executePreview(ctx, execution.PreviewRequest{HTTPPayload: payload, ExpectedTarget: target})
}

func executePreview(ctx context.Context, input execution.PreviewRequest) (*execution.HTTPResponse, error) {
	if input.FollowRedirects {
		return nil, errors.New("turn off Follow redirects or use Local Agent")
	}
	origin := js.Global().Get("location").Get("origin").String()
	body, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("could not encode HTTP request")
	}
	controller := js.Global().Get("AbortController").New()
	// Keep the signal connected until the response stream closes. The Go WASM
	// HTTP transport only observes cancellation until fetch returns headers.
	stop := context.AfterFunc(ctx, func() { controller.Call("abort") })
	stream := &browserBody{abort: func() {
		stop()
		controller.Call("abort")
	}}
	response, err := awaitPromise(js.Global().Call("fetch", origin+"/api/preview", map[string]any{
		"method":  http.MethodPost,
		"headers": map[string]any{"Content-Type": "application/json", "X-ProtoVoy-Preview": "1"},
		"body":    string(body), "signal": controller.Get("signal"),
		"mode": "same-origin", "redirect": "error",
	}))
	if err != nil {
		stream.Close()
		return nil, fmt.Errorf("could not reach execution API")
	}
	code := response.Get("status").Int()
	switch code {
	case http.StatusUnauthorized:
		stream.Close()
		return nil, errors.New("sign in to the hosted workbench and try again")
	case http.StatusForbidden:
		stream.Close()
		return nil, errors.New("hosted execution was denied; open the configured workbench address")
	case http.StatusServiceUnavailable:
		stream.Close()
		return nil, errors.New("no hosted HTTP endpoint is configured; pair a Local Agent to send this request")
	case http.StatusGatewayTimeout:
		stream.Close()
		return nil, context.DeadlineExceeded
	case http.StatusRequestEntityTooLarge:
		stream.Close()
		return nil, fmt.Errorf("request exceeds the server's 1 MiB limit")
	case http.StatusBadRequest:
		stream.Close()
		return nil, fmt.Errorf("server rejected the HTTP request")
	case http.StatusConflict:
		stream.Close()
		return nil, errors.New("request URL is not in the hosted HTTP allowlist")
	case http.StatusTooManyRequests:
		stream.Close()
		return nil, errors.New("hosted script capacity is busy; try again")
	case http.StatusUnprocessableEntity:
		phase := response.Get("headers").Call("get", "X-ProtoVoy-Script-Phase").String()
		stream.Close()
		if phase == "pre-request" || phase == "post-response" {
			return nil, fmt.Errorf("hosted %s script failed", phase)
		}
		return nil, errors.New("hosted request script failed")
	}
	headers := response.Get("headers")
	status, err := strconv.Atoi(headers.Call("get", "X-ProtoVoy-Status").String())
	bodyStream := response.Get("body")
	if code != http.StatusOK || err != nil || bodyStream.IsNull() {
		stream.Close()
		return nil, fmt.Errorf("server could not execute request")
	}
	stream.reader = bodyStream.Call("getReader")
	contentType := headers.Call("get", "X-ProtoVoy-Content-Type")
	responseHeaders := make(http.Header)
	metadata := headers.Call("get", execution.ResponseHeadersMetadata)
	if !metadata.IsNull() && len(metadata.String()) <= 8192 {
		var pairs [][2]string
		if data, err := base64.StdEncoding.DecodeString(metadata.String()); err == nil && json.Unmarshal(data, &pairs) == nil {
			for _, pair := range pairs {
				responseHeaders.Add(pair[0], pair[1])
			}
		}
	}
	if !contentType.IsNull() && responseHeaders.Get("Content-Type") == "" {
		responseHeaders.Set("Content-Type", contentType.String())
	}
	return &execution.HTTPResponse{StatusCode: status, Headers: responseHeaders, Body: stream}, nil
}

type browserBody struct {
	reader js.Value
	chunk  js.Value
	offset int
	abort  func()
	closed sync.Once
}

func (body *browserBody) Read(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	for body.chunk.IsUndefined() || body.offset == body.chunk.Length() {
		result, err := awaitPromise(body.reader.Call("read"))
		if err != nil {
			return 0, err
		}
		if result.Get("done").Bool() {
			return 0, io.EOF
		}
		body.chunk, body.offset = result.Get("value"), 0
	}
	end := min(body.offset+len(data), body.chunk.Length())
	n := js.CopyBytesToGo(data, body.chunk.Call("subarray", body.offset, end))
	body.offset += n
	return n, nil
}

func (body *browserBody) Close() error {
	body.closed.Do(body.abort)
	return nil
}

func awaitPromise(promise js.Value) (js.Value, error) {
	type outcome struct {
		value js.Value
		err   error
	}
	result := make(chan outcome, 1)
	resolve := js.FuncOf(func(_ js.Value, args []js.Value) any {
		result <- outcome{value: args[0]}
		return nil
	})
	reject := js.FuncOf(func(_ js.Value, _ []js.Value) any {
		result <- outcome{err: errors.New("browser network operation failed")}
		return nil
	})
	defer resolve.Release()
	defer reject.Release()
	promise.Call("then", resolve, reject)
	value := <-result
	return value.value, value.err
}
