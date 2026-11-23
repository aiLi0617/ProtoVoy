package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"syscall/js"

	"protovoy.dev/protovoy/internal/execution"
)

func exchangeDatagram(ctx context.Context, payload execution.SocketPayload) ([]byte, error) {
	var value execution.SocketResponse
	if err := fetchExecutionResponse(ctx, "/api/socket", "socket", payload, &value); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(value.Data)
}

func fetchExecutionResponse(ctx context.Context, path, label string, payload, value any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	controller := js.Global().Get("AbortController").New()
	stop := context.AfterFunc(ctx, func() { controller.Call("abort") })
	defer stop()
	defer controller.Call("abort")
	origin := js.Global().Get("location").Get("origin").String()
	response, err := awaitPromise(js.Global().Call("fetch", origin+path, map[string]any{
		"method": "POST", "mode": "same-origin", "redirect": "error", "body": string(body), "signal": controller.Get("signal"),
		"headers": map[string]any{"Content-Type": "application/json", "X-ProtoVoy-Preview": "1"},
	}))
	if err != nil {
		return errors.New("could not reach " + label + " execution API")
	}
	status := response.Get("status").Int()
	if status == 504 {
		return context.DeadlineExceeded
	}
	if status != 200 {
		return errors.New(label + " request rejected or failed on server")
	}
	text, err := awaitPromise(response.Call("text"))
	if err != nil {
		return err
	}
	if json.Unmarshal([]byte(text.String()), value) != nil {
		return errors.New("invalid " + label + " response")
	}
	return nil
}
