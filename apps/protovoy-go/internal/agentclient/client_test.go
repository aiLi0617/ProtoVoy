package agentclient

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"protovoy.dev/protovoy/internal/execution"
)

func TestPairOnlySendsTokenAfterLoopbackHealth(t *testing.T) {
	calls := 0
	do := func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != DefaultAddress || r.URL.Path != "/health" || r.Header.Get("Authorization") != "" {
			t.Fatal("pairing credential was sent before checking the Agent")
		}
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	}
	if _, err := Pair(context.Background(), "192.0.2.1:39217", "fixture", do); err == nil || calls != 0 {
		t.Fatal("non-loopback Agent address was not rejected locally")
	}
	if _, err := Pair(context.Background(), "", "fixture", do); err == nil || calls != 1 {
		t.Fatal("invalid health response did not prevent pairing")
	}
}

func TestExecutionStreamCombinesSplitBytesAndRequiresTerminalEvent(t *testing.T) {
	const stream = "event: execution\ndata: {\"type\":\"response_meta\",\"status\":200,\"headers\":[[\"Set-Cookie\",\"session=fixture\"],[\"X-Fixture\",\"first\"],[\"X-Fixture\",\"second\"]]}\n\n" +
		"event: execution\ndata: {\"type\":\"response_chunk\",\"data_base64\":\"5Lw=\"}\n\n" +
		"event: execution\ndata: {\"type\":\"response_chunk\",\"data_base64\":\"oA==\"}\n\n"
	updates := 0
	wantHeaders := [][2]string{{"Set-Cookie", "[redacted]"}, {"X-Fixture", "first"}, {"X-Fixture", "second"}}
	got, finished, err := readEvents(strings.NewReader(stream+"event: execution\ndata: {\"type\":\"completed\"}\n\n"), func(response Response) {
		updates++
		if !slices.Equal(response.Headers, wantHeaders) {
			t.Fatalf("unexpected response metadata: %v", response.Headers)
		}
	})
	if err != nil || !finished || updates != 3 || got.Status != 200 || got.Preview != "传" || got.PreviewBytes != 3 {
		t.Fatalf("unexpected execution response: %+v, %v", got, err)
	}
	if !slices.Equal(got.Headers, wantHeaders) {
		t.Fatal("response metadata was lost after completion")
	}
	if string(got.RawPreview) != "传" {
		t.Fatal("raw preview did not retain the original response bytes")
	}
	_, finished, err = readEvents(strings.NewReader(stream), func(Response) {})
	if err == nil || finished {
		t.Fatal("truncated event stream appeared complete")
	}
}

func TestExecutionStreamKeepsPreviewBounded(t *testing.T) {
	stream := "data: {\"type\":\"response_meta\",\"status\":200}\n\n" +
		"data: {\"type\":\"response_chunk\",\"done\":true,\"preview\":\"" + strings.Repeat("x", execution.PreviewLimit+1) + "\"}\n\n" +
		"data: {\"type\":\"completed\"}\n\n"
	got, finished, err := readEvents(strings.NewReader(stream), func(Response) {})
	if err != nil || !finished || !got.Truncated || got.PreviewBytes != execution.PreviewLimit || len(got.Preview) != execution.PreviewLimit {
		t.Fatalf("preview bound was not applied: %v", err)
	}
}
