package main

import (
	"context"

	"io"
	"net/http"
	"syscall/js"
)

func doAgentRequest(request *http.Request) (*http.Response, error) {
	return fetchStreamingResponse(request, "cors", "omit")
}

func fetchStreamingResponse(request *http.Request, mode, credentials string) (*http.Response, error) {
	headers := js.Global().Get("Headers").New()
	for key, values := range request.Header {
		for _, value := range values {
			headers.Call("append", key, value)
		}
	}
	controller := js.Global().Get("AbortController").New()
	stop := context.AfterFunc(request.Context(), func() { controller.Call("abort") })
	body := &browserBody{abort: func() {
		stop()
		controller.Call("abort")
	}}
	options := map[string]any{
		"method": request.Method, "headers": headers, "mode": mode, "credentials": credentials,
		"redirect": "error", "signal": controller.Get("signal"),
	}
	if request.Body != nil {
		data, err := io.ReadAll(request.Body)
		request.Body.Close()
		if err != nil {
			body.Close()
			return nil, err
		}
		if len(data) != 0 {
			options["body"] = string(data)
		}
	}
	response, err := awaitPromise(js.Global().Call("fetch", request.URL.String(), options))
	if err != nil {
		body.Close()
		return nil, err
	}
	responseBody := response.Get("body")
	responseHeaders := make(http.Header)
	for _, name := range []string{"Content-Type", "ETag"} {
		if value := response.Get("headers").Call("get", name); !value.IsNull() {
			responseHeaders.Set(name, value.String())
		}
	}
	if responseBody.IsNull() {
		body.Close()
		return &http.Response{StatusCode: response.Get("status").Int(), Header: responseHeaders, Body: http.NoBody}, nil
	}
	body.reader = responseBody.Call("getReader")
	return &http.Response{StatusCode: response.Get("status").Int(), Header: responseHeaders, Body: body}, nil
}
