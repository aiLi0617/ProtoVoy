package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"protovoy.dev/protovoy/internal/agentclient"
	"protovoy.dev/protovoy/internal/agenthistory"
	"protovoy.dev/protovoy/internal/execution"
)

func TestHistoryReopensAndPrunesOldestRecord(t *testing.T) {
	dir := t.TempDir()
	started := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range historyLimit {
		record := agenthistory.Record{
			ID: fmt.Sprintf("a0000000-0000-4000-8000-%012x", i), RequestID: "a0000000-0000-4000-8000-000000000099",
			ProtocolID: "http", State: "completed", StartedAt: started, FinishedAt: started.Add(time.Duration(i) * time.Second),
		}
		data, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		name := "execution-" + record.FinishedAt.Format(historyStamp) + "-" + record.ID + ".json"
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	history, err := openHistory(dir)
	if err != nil {
		t.Fatal(err)
	}
	newest := agenthistory.Record{
		ID: "b0000000-0000-4000-8000-000000000001", RequestID: "b0000000-0000-4000-8000-000000000002",
		ProtocolID: "redis", State: "failed", StartedAt: started, FinishedAt: started.Add(time.Hour),
	}
	err = history.save(newest, nil)
	closeErr := history.root.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("could not save and close history: %v, %v", err, closeErr)
	}
	reopened, err := openHistory(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.root.Close()
	names, err := reopened.names()
	if err != nil || len(names) != historyLimit {
		t.Fatalf("history retention: %d files, %v", len(names), err)
	}
	items, err := reopened.list(historyFilter{limit: 1})
	if err != nil || len(items) != 1 || items[0].ID != newest.ID || items[0].State != "failed" {
		t.Fatalf("newest execution missing after reopening: %v", err)
	}
	items, err = reopened.list(historyFilter{id: "a0000000-0000-4000-8000-000000000000", limit: 1})
	if err != nil || len(items) != 0 {
		t.Fatal("oldest execution was retained")
	}
}

func TestHistoryResponseCaptureIsBoundedAndAuthenticated(t *testing.T) {
	agent := testAgentWithResponseLimit(t, 3)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		if r.URL.Path != "/empty" {
			_, _ = w.Write([]byte{0, 1, 255, 7})
		}
	}))
	defer upstream.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := agentclient.Pair(ctx, agent.Listener.Addr().String(), pairingFixture, agent.Client().Do)
	if err != nil {
		t.Fatal(err)
	}
	requestID := agentclient.NewRequestID()
	snapshotID := agentclient.NewRequestID()
	executionContext := agentclient.WithHistorySnapshotID(agentclient.WithSavedRequestID(ctx, requestID), snapshotID)
	for _, test := range []struct {
		path      string
		want      []byte
		truncated bool
	}{
		{path: "/", want: []byte{0, 1, 255}, truncated: true},
		{path: "/empty"},
	} {
		if _, err := client.RunHTTP(executionContext, upstream.URL+test.path, execution.HTTPPayload{Method: http.MethodGet}, func(agentclient.Response) {}); err != nil {
			t.Fatal(err)
		}
		items, err := client.History(ctx)
		if err != nil || len(items) == 0 || items[0].RequestID != requestID || items[0].SnapshotID != snapshotID || items[0].ResponseBodyBytes == nil ||
			*items[0].ResponseBodyBytes != len(test.want) || items[0].ResponseBodyTruncated != test.truncated {
			t.Fatalf("invalid capture metadata: %v", err)
		}
		url := agent.URL + "/v1/history/" + items[0].ID + "/body"
		anonymous, err := agent.Client().Get(url)
		if err != nil {
			t.Fatal(err)
		}
		anonymous.Body.Close()
		if anonymous.StatusCode != http.StatusUnauthorized {
			t.Fatal("anonymous request could read an archived response")
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+pairingFixture)
		response, err := agent.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK || !bytes.Equal(data, test.want) ||
			response.Header.Get("Content-Type") != "application/octet-stream" || response.Header.Get("Content-Disposition") != "attachment" {
			t.Fatalf("archived binary response changed: HTTP %d, %v", response.StatusCode, err)
		}
	}
}
