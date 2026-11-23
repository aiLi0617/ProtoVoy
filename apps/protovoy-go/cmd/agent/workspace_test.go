package main

import (
	"bytes"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"protovoy.dev/protovoy/internal/project"
)

func TestConcurrentWorkspaceUpdatesAllowOneWinner(t *testing.T) {
	store, err := openHistory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.root.Close()
	agent := &agentService{history: store}
	initial, err := project.EncodeWorkspace(project.Workspace{Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	agent.workspace(response, httptest.NewRequest(http.MethodPut, "/v1/workspace", bytes.NewReader(initial)))
	if response.Code != http.StatusNoContent {
		t.Fatal("could not create initial workspace")
	}
	response = httptest.NewRecorder()
	agent.workspace(response, httptest.NewRequest(http.MethodGet, "/v1/workspace", nil))
	etag := response.Header().Get("ETag")
	start := make(chan struct{})
	results := make(chan int, 2)
	var writers sync.WaitGroup
	for _, name := range []string{"first", "second"} {
		data, err := project.EncodeWorkspace(project.Workspace{Version: 1, Draft: project.Draft{Name: name}})
		if err != nil {
			t.Fatal(err)
		}
		writers.Go(func() {
			<-start
			request := httptest.NewRequest(http.MethodPut, "/v1/workspace", bytes.NewReader(data))
			request.Header.Set("If-Match", etag)
			result := httptest.NewRecorder()
			agent.workspace(result, request)
			results <- result.Code
		})
	}
	close(start)
	writers.Wait()
	close(results)
	counts := make(map[int]int)
	for status := range results {
		counts[status]++
	}
	if counts[http.StatusNoContent] != 1 || counts[http.StatusPreconditionFailed] != 1 {
		t.Fatalf("concurrent updates returned %v", counts)
	}
}

func TestWorkspaceAcceptsLiveSessionAndRejectsExpiredSession(t *testing.T) {
	store, err := openHistory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.root.Close()
	const origin = "http://127.0.0.1:39300"
	agent, err := newAgent("127.0.0.1:39217", "fixture-pairing-token-for-workspace", origin, store)
	if err != nil {
		t.Fatal(err)
	}
	const token = "fixture-session-token"
	key := sha256.Sum256([]byte(token))
	agent.sessions[key] = session{origin: origin, expires: time.Now().Add(time.Minute)}
	api := agent.routes()
	call := func() int {
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:39217/v1/workspace", nil)
		request.RemoteAddr = "127.0.0.1:45000"
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Origin", origin)
		response := httptest.NewRecorder()
		api.ServeHTTP(response, request)
		return response.Code
	}
	if status := call(); status != http.StatusNotFound {
		t.Fatalf("live session did not reach workspace handler: %d", status)
	}
	agent.sessions[key] = session{origin: origin, expires: time.Now().Add(-time.Minute)}
	if status := call(); status != http.StatusUnauthorized {
		t.Fatalf("expired session reached workspace handler: %d", status)
	}
}

func TestWorkspaceRejectsCorruptStoredDocument(t *testing.T) {
	store, err := openHistory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.root.Close()
	file, err := store.root.OpenFile(workspaceFile, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteString(`{"version":1`)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	agent := &agentService{history: store}
	response := httptest.NewRecorder()
	agent.workspace(response, httptest.NewRequest(http.MethodGet, "/v1/workspace", nil))
	if response.Code != http.StatusInternalServerError || response.Header().Get("ETag") != "" {
		t.Fatalf("corrupt workspace returned as valid: %d %v", response.Code, response.Header())
	}
}

func TestWorkspaceRoutesRequireAuthenticationAndAllowBrowserMethods(t *testing.T) {
	agent, err := newAgent("127.0.0.1:39217", "fixture-pairing-token-for-workspace", "http://127.0.0.1:39300", nil)
	if err != nil {
		t.Fatal(err)
	}
	api := agent.routes()
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		request := httptest.NewRequest(method, "http://127.0.0.1:39217/v1/workspace", nil)
		request.RemoteAddr = "127.0.0.1:45000"
		request.Header.Set("Origin", "http://127.0.0.1:39300")
		response := httptest.NewRecorder()
		api.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous %s returned %d", method, response.Code)
		}
		request = httptest.NewRequest(http.MethodOptions, "http://127.0.0.1:39217/v1/workspace", nil)
		request.RemoteAddr = "127.0.0.1:45000"
		request.Header.Set("Origin", "http://127.0.0.1:39300")
		request.Header.Set("Access-Control-Request-Method", method)
		response = httptest.NewRecorder()
		api.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent || !strings.Contains(response.Header().Get("Access-Control-Allow-Methods"), method) {
			t.Fatalf("browser preflight for %s failed: %d %v", method, response.Code, response.Header())
		}
	}
}

func TestAuthenticatedWorkspaceRoutes(t *testing.T) {
	store, err := openHistory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.root.Close()
	const token = "fixture-pairing-token-for-workspace"
	agent, err := newAgent("127.0.0.1:39217", token, "http://127.0.0.1:39300", store)
	if err != nil {
		t.Fatal(err)
	}
	api := agent.routes()
	valid, err := project.EncodeWorkspace(project.Workspace{Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	call := func(method string, body []byte, header, version string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, "http://127.0.0.1:39217/v1/workspace", bytes.NewReader(body))
		request.RemoteAddr = "127.0.0.1:45000"
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Origin", "http://127.0.0.1:39300")
		if header != "" {
			request.Header.Set(header, version)
		}
		response := httptest.NewRecorder()
		api.ServeHTTP(response, request)
		return response
	}
	if response := call(http.MethodPut, valid, "If-None-Match", "*"); response.Code != http.StatusNoContent {
		t.Fatalf("create failed: %d %s", response.Code, response.Body)
	}
	read := call(http.MethodGet, nil, "", "")
	etag := read.Header().Get("ETag")
	if read.Code != http.StatusOK || etag == "" || read.Header().Get("Access-Control-Expose-Headers") != "ETag" {
		t.Fatalf("authenticated read failed: %d %v", read.Code, read.Header())
	}
	if response := call(http.MethodPut, valid, "If-Match", etag); response.Code != http.StatusNoContent {
		t.Fatalf("versioned update failed: %d", response.Code)
	}
	if response := call(http.MethodDelete, nil, "If-Match", etag); response.Code != http.StatusNoContent {
		t.Fatalf("versioned delete failed: %d", response.Code)
	}
	if response := call(http.MethodGet, nil, "", ""); response.Code != http.StatusNotFound {
		t.Fatalf("deleted workspace still present: %d", response.Code)
	}
}

func TestWorkspaceSurvivesStorageReopen(t *testing.T) {
	dir := t.TempDir()
	store, err := openHistory(dir)
	if err != nil {
		t.Fatal(err)
	}
	agent := &agentService{history: store}
	value := project.Workspace{Version: 1, Draft: project.Draft{Name: "Saved draft", Method: "GET", Target: "http://example.test/"}}
	data, err := project.EncodeWorkspace(value)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	agent.workspace(response, httptest.NewRequest(http.MethodPut, "/v1/workspace", bytes.NewReader(data)))
	if response.Code != http.StatusNoContent {
		t.Fatalf("save failed: %d %s", response.Code, response.Body)
	}
	if err := store.root.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = openHistory(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.root.Close()
	agent = &agentService{history: store}
	response = httptest.NewRecorder()
	agent.workspace(response, httptest.NewRequest(http.MethodGet, "/v1/workspace", nil))
	restored, err := project.DecodeWorkspace(response.Body.Bytes())
	if response.Code != http.StatusOK || err != nil || restored.Draft.Name != value.Draft.Name || restored.Draft.Target != value.Draft.Target {
		t.Fatalf("workspace was not restored: %d %v %v", response.Code, restored, err)
	}
}

func TestAgentWorkspaceLifecycle(t *testing.T) {
	store, err := openHistory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.root.Close()
	agent := &agentService{history: store}
	call := func(method string, body []byte, status int) []byte {
		t.Helper()
		response := httptest.NewRecorder()
		agent.workspace(response, httptest.NewRequest(method, "/v1/workspace", bytes.NewReader(body)))
		if response.Code != status {
			t.Fatalf("%s returned %d: %s", method, response.Code, response.Body)
		}
		return response.Body.Bytes()
	}
	call(http.MethodGet, nil, http.StatusNotFound)
	valid, err := project.EncodeWorkspace(project.Workspace{Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	call(http.MethodPut, valid, http.StatusNoContent)
	create := httptest.NewRequest(http.MethodPut, "/v1/workspace", bytes.NewReader(valid))
	create.Header.Set("If-None-Match", "*")
	createResponse := httptest.NewRecorder()
	agent.workspace(createResponse, create)
	if createResponse.Code != http.StatusPreconditionFailed {
		t.Fatalf("conditional creation overwrote workspace: %d", createResponse.Code)
	}
	before := append([]byte(nil), call(http.MethodGet, nil, http.StatusOK)...)
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		request := httptest.NewRequest(method, "/v1/workspace", bytes.NewReader(valid))
		request.Header.Set("If-Match", "\"stale\"")
		response := httptest.NewRecorder()
		agent.workspace(response, request)
		if response.Code != http.StatusPreconditionFailed {
			t.Fatalf("stale %s returned %d", method, response.Code)
		}
	}
	call(http.MethodPut, []byte(`{"version":99}`), http.StatusBadRequest)
	if after := call(http.MethodGet, nil, http.StatusOK); !bytes.Equal(before, after) {
		t.Fatal("invalid input replaced saved workspace")
	}
	call(http.MethodPut, bytes.Repeat([]byte("x"), workspaceLimit+1), http.StatusRequestEntityTooLarge)
	call(http.MethodDelete, nil, http.StatusNoContent)
	call(http.MethodGet, nil, http.StatusNotFound)
	call(http.MethodDelete, nil, http.StatusNoContent)
}
