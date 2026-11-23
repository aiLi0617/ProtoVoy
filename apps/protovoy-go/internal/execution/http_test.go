package execution

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPExecuteStreamsAndCancels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil || r.Method != http.MethodPost || string(body) != `{"a":1}` ||
			len(r.Header.Values("X-Tag")) != 2 {
			t.Errorf("upstream did not receive the expected request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "first")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response, err := (HTTPClient{}).Execute(ctx, HTTPRequest{
		Method: http.MethodPost, Target: server.URL, Body: `{"a":1}`,
		Headers: [][2]string{{"X-Tag", "one"}, {"X-Tag", "two"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated || response.Headers.Get("Content-Type") != "text/plain" {
		t.Fatalf("unexpected response metadata: %d", response.StatusCode)
	}
	chunk := make([]byte, len("first"))
	if _, err := io.ReadFull(response.Body, chunk); err != nil || string(chunk) != "first" {
		t.Fatalf("first chunk = %q, error = %v", chunk, err)
	}
	cancel()
	if _, err := response.Body.Read(chunk); !errors.Is(err, context.Canceled) {
		t.Fatalf("response body after cancellation: %v", err)
	}
}
