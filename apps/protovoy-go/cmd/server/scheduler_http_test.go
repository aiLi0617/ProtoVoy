package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/zalando/go-keyring"
	"protovoy.dev/protovoy/internal/project"
	"protovoy.dev/protovoy/internal/secrets"
)

func TestScheduledHTTPCredentialsRequireOptInAndExactBinding(t *testing.T) {
	keyring.MockInit()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer fixture" {
			t.Error("bound credential was not applied")
		}
		fmt.Fprint(w, "fixture")
	}))
	defer server.Close()
	if err := secrets.PutHTTP("scheduled-fixture", server.URL, "Authorization", "Bearer fixture"); err != nil {
		t.Fatal(err)
	}
	request := project.Request{Method: "GET", URL: server.URL, HTTPSecretReference: "scheduled-fixture", HTTPSecretHeader: "Authorization"}
	if result, err := scheduledHTTPExecutor(server.URL, nil)(t.Context(), request); err == nil || result.Success {
		t.Fatal("keychain access enabled by default")
	}
	execute := scheduledHTTPExecutor(server.URL, map[string]struct{}{server.URL + "/other": {}}, true)
	if result, err := execute(t.Context(), request); err != nil || !result.Success {
		t.Fatalf("bound request failed: %+v %v", result, err)
	}
	before := calls.Load()
	for _, change := range []func(*project.Request){
		func(r *project.Request) { r.URL += "/other" },
		func(r *project.Request) { r.HTTPSecretHeader = "X-Api-Key" },
		func(r *project.Request) { r.HTTPSecretReference = "missing" },
		func(r *project.Request) { r.FollowRedirects = true },
		func(r *project.Request) { r.PreScripts = []string{"request.target += '/other';"} },
		func(r *project.Request) { r.Headers = map[string]string{"authorization": "override"} },
	} {
		input := request
		change(&input)
		if result, err := execute(t.Context(), input); err == nil || result.Success {
			t.Fatal("invalid binding accepted")
		}
	}
	if calls.Load() != before {
		t.Fatal("invalid credential request reached upstream")
	}
	data, err := project.CaptureHistoryRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "Bearer fixture") {
		t.Fatal("credential leaked into schedule snapshot")
	}
}

func TestScheduledHTTPScriptsRevalidateAndAssert(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "POST" {
			t.Error("script method was not applied")
		}
		fmt.Fprint(w, "fixture")
	}))
	defer upstream.Close()
	allowedTarget := upstream.URL + "/allowed"
	execute := scheduledHTTPExecutor(upstream.URL, map[string]struct{}{allowedTarget: {}})
	encodedTarget, _ := json.Marshal(allowedTarget)
	request := project.Request{Method: "GET", URL: upstream.URL, ResponseBodyContains: "fixture",
		PreScripts:  []string{"request.target=" + string(encodedTarget) + "; request.payload.method='POST'; variables.marker='ready';"},
		PostScripts: []string{"assert(response.status===200); assert(variables.marker==='ready'); assert(response.body==='fixture');"},
	}
	result, err := execute(t.Context(), request)
	if err != nil || !result.Success {
		t.Fatalf("scripts failed: %+v %v", result, err)
	}
	request.PostScripts = []string{"assert(false, 'secret diagnostic');"}
	if result, err = execute(t.Context(), request); err == nil || result.Success || err.Error() != "scheduled post-response script failed" {
		t.Fatalf("post failure: %+v %v", result, err)
	}
	before := calls.Load()
	for _, script := range []string{
		"request.target += '/forbidden';",
		"request.payload.followRedirects=true;",
		"request.payload.method='CONNECT';",
		"throw new Error('secret diagnostic');",
	} {
		request.PreScripts = []string{script}
		if result, err = execute(t.Context(), request); err == nil || result.Success {
			t.Fatalf("unsafe script accepted: %s", script)
		}
	}
	if calls.Load() != before {
		t.Fatal("unsafe script reached upstream")
	}
}

func TestScheduledHTTPPolicyAndAssertions(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(404)
		fmt.Fprint(w, "fixture result")
	}))
	defer upstream.Close()
	execute := scheduledHTTPExecutor(upstream.URL, nil)
	request := project.Request{Method: "GET", URL: upstream.URL, ExpectedHTTPStatus: 404, ResponseBodyContains: "{{expected}}", Variables: map[string]string{"expected": "fixture"}}
	result, err := execute(t.Context(), request)
	if err != nil || !result.Success || result.Status != 404 {
		t.Fatalf("expected 404: %+v %v", result, err)
	}
	request.ExpectedHTTPStatus = 0
	result, err = execute(t.Context(), request)
	if err != nil || result.Success {
		t.Fatalf("unexpected 404 passed: %+v %v", result, err)
	}
	request.ResponseBodyContains = "missing"
	if result, err = execute(t.Context(), request); err == nil || result.Success {
		t.Fatal("missing body assertion passed")
	}
	before := calls.Load()
	for _, change := range []func(*project.Request){
		func(r *project.Request) { r.URL += "/unconfigured" },
		func(r *project.Request) { r.FollowRedirects = true },
		func(r *project.Request) { r.ResponseBodyContains = "{{missing}}" },
		func(r *project.Request) { r.PreScripts = []string{"throw new Error('unexpected')"} },
	} {
		input := request
		change(&input)
		if _, err := execute(t.Context(), input); err == nil {
			t.Fatal("invalid request accepted")
		}
	}
	if calls.Load() != before {
		t.Fatal("rejected request reached upstream")
	}
}
