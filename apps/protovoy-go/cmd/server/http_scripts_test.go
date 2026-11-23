package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"protovoy.dev/protovoy/internal/execution"
)

func TestHostedScriptsMutateAllowedRequestsAndPreserveBinaryBodies(t *testing.T) {
	var hits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("X-Script") != "ready" {
			t.Error("pre-request header was not applied")
		}
		if r.URL.Path == "/binary" {
			body, _ := io.ReadAll(r.Body)
			if !bytes.Equal(body, []byte{0, 255, 1}) {
				t.Error("binary request was corrupted by script serialization")
			}
			_, _ = w.Write(body)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("scripted response"))
	}))
	defer upstream.Close()
	const addr = "127.0.0.1:39300"
	api := handler(loopbackAccess(addr), t.TempDir(), upstream.URL, "", socketTargets{AdditionalHTTP: map[string]struct{}{upstream.URL + "/jobs": {}, upstream.URL + "/binary": {}}})
	for _, binary := range []bool{false, true} {
		input := execution.PreviewRequest{HTTPPayload: execution.HTTPPayload{Method: "POST"}, ExpectedTarget: upstream.URL,
			Variables:   map[string]string{"step": "ready"},
			PreScripts:  []string{`request.payload.headers.push(['X-Script',variables.step]);`},
			PostScripts: []string{`assert(response.status===201 && response.body==='scripted response' && variables.step==='ready');`}}
		if binary {
			input.BodyEncoding, input.Body = "base64", "AP8B"
			input.PreScripts = append(input.PreScripts, `assert(request.payload.bodyEncoding==='base64' && request.payload.body==='AP8B'); request.target+='/binary';`)
			input.PostScripts = []string{`assert(response.bodyEncoding==='base64' && response.body==='AP8B' && response.bytesReceived===3);`}
		} else {
			input.PreScripts = append(input.PreScripts, `request.target+='/jobs'; variables.step='ready';`)
		}
		data, _ := json.Marshal(input)
		request := httptest.NewRequest(http.MethodPost, "http://"+addr+"/api/preview", bytes.NewReader(data))
		request.Header.Set("Origin", "http://"+addr)
		request.Header.Set("X-ProtoVoy-Preview", "1")
		response := httptest.NewRecorder()
		api.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("scripted request failed: HTTP %d, %s", response.Code, response.Body.String())
		}
		if binary && !bytes.Equal(response.Body.Bytes(), []byte{0, 255, 1}) || !binary && response.Body.String() != "scripted response" {
			t.Fatal("scripted response body was corrupted")
		}
	}
	if hits.Load() != 2 {
		t.Fatal("unexpected upstream request count")
	}
}

func TestHostedScriptFailuresCannotBypassTargetPolicyOrExposeResponse(t *testing.T) {
	var hits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("X-Private-Fixture", "upstream-private-value")
		_, _ = w.Write([]byte("upstream-private-body"))
	}))
	defer upstream.Close()
	const addr = "127.0.0.1:39300"
	api := handler(loopbackAccess(addr), t.TempDir(), upstream.URL, "")
	for _, test := range []struct {
		name      string
		pre, post []string
		stream    bool
		status    int
		phase     string
		hits      int32
	}{
		{"pre assertion", []string{`throw new Error('private-script-value')`}, nil, false, 422, "pre-request", 0},
		{"target escape", []string{`request.target='http://127.0.0.1:1/not-allowed'`}, nil, false, 409, "", 0},
		{"redirect escape", []string{`request.payload.followRedirects=true`}, nil, false, 400, "", 0},
		{"unsupported method", []string{`request.payload.method='CONNECT'`}, nil, false, 400, "", 0},
		{"too many scripts", make([]string, 33), nil, false, 400, "", 0},
		{"streaming", []string{`variables.x='y'`}, nil, true, 400, "", 0},
		{"post assertion", nil, []string{`throw new Error(response.body)`}, false, 422, "post-response", 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := hits.Load()
			input := execution.PreviewRequest{HTTPPayload: execution.HTTPPayload{Method: "GET", EventStream: test.stream}, PreScripts: test.pre, PostScripts: test.post}
			data, _ := json.Marshal(input)
			request := httptest.NewRequest(http.MethodPost, "http://"+addr+"/api/preview", bytes.NewReader(data))
			request.Header.Set("Origin", "http://"+addr)
			request.Header.Set("X-ProtoVoy-Preview", "1")
			response := httptest.NewRecorder()
			api.ServeHTTP(response, request)
			if response.Code != test.status || response.Header().Get("X-ProtoVoy-Script-Phase") != test.phase || hits.Load()-before != test.hits {
				t.Fatalf("unexpected script failure: HTTP %d, phase %s, upstream hits %d", response.Code, response.Header().Get("X-ProtoVoy-Script-Phase"), hits.Load()-before)
			}
			if strings.Contains(response.Body.String(), "private") || response.Header().Get(execution.ResponseHeadersMetadata) != "" || response.Header().Get("X-ProtoVoy-Status") != "" {
				t.Fatal("script failure exposed script or upstream content")
			}
		})
	}
}

func TestHostedScriptsBoundConcurrencyWithoutBlockingOrdinaryHTTP(t *testing.T) {
	entered, release := make(chan struct{}, 2), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Hold") == "yes" {
			entered <- struct{}{}
			select {
			case <-release:
			case <-r.Context().Done():
			}
		}
		_, _ = w.Write([]byte("ready"))
	}))
	defer upstream.Close()
	defer unblock()
	const addr = "127.0.0.1:39300"
	api := handler(loopbackAccess(addr), t.TempDir(), upstream.URL, "")
	call := func(scripted bool) int {
		input := execution.PreviewRequest{HTTPPayload: execution.HTTPPayload{Method: "GET"}}
		if scripted {
			input.PreScripts = []string{`request.payload.headers.push(['X-Hold','yes'])`}
		}
		data, _ := json.Marshal(input)
		request := httptest.NewRequest(http.MethodPost, "http://"+addr+"/api/preview", bytes.NewReader(data))
		request.Header.Set("Origin", "http://"+addr)
		request.Header.Set("X-ProtoVoy-Preview", "1")
		response := httptest.NewRecorder()
		api.ServeHTTP(response, request)
		return response.Code
	}
	done := make(chan int, 2)
	for range 2 {
		go func() { done <- call(true) }()
	}
	for range 2 {
		select {
		case <-entered:
		case <-time.After(20 * time.Second):
			t.Fatal("scripted executions did not enter upstream")
		}
	}
	if status := call(true); status != http.StatusTooManyRequests {
		t.Fatalf("excess scripted execution got HTTP %d", status)
	}
	if status := call(false); status != http.StatusOK {
		t.Fatalf("ordinary HTTP was blocked: HTTP %d", status)
	}
	unblock()
	for range 2 {
		if status := <-done; status != http.StatusOK {
			t.Fatalf("scripted execution failed: HTTP %d", status)
		}
	}
	if status := call(true); status != http.StatusOK {
		t.Fatalf("script slot was not released: HTTP %d", status)
	}
}

func TestHostedPostScriptResponseLimitDoesNotLeakBody(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, strings.Repeat("s", (1<<20)+1)) }))
	defer upstream.Close()
	const addr = "127.0.0.1:39300"
	api := handler(loopbackAccess(addr), t.TempDir(), upstream.URL, "")
	input := execution.PreviewRequest{HTTPPayload: execution.HTTPPayload{Method: "GET"}, PostScripts: []string{`assert(true)`}}
	data, _ := json.Marshal(input)
	request := httptest.NewRequest(http.MethodPost, "http://"+addr+"/api/preview", bytes.NewReader(data))
	request.Header.Set("Origin", "http://"+addr)
	request.Header.Set("X-ProtoVoy-Preview", "1")
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity || response.Header().Get("X-ProtoVoy-Script-Phase") != "post-response" || response.Header().Get("X-ProtoVoy-Status") != "" || response.Body.Len() > 100 {
		t.Fatal("oversized scripted response was exposed as a successful preview")
	}
}
