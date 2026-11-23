//go:build !js

package scheduleclient

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"protovoy.dev/protovoy/internal/project"
	"protovoy.dev/protovoy/internal/scheduler"
)

func TestClientManagesPersistedSchedules(t *testing.T) {
	store, err := scheduler.Open(t.Context(), filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var origin string
	server := httptest.NewServer(store.Handler(func(r *http.Request) bool {
		user, password, ok := r.BasicAuth()
		return ok && user == "operator" && password == "fixture" && r.Header.Get("Origin") == origin && r.Header.Get("X-ProtoVoy-Scheduler") == "1"
	}))
	defer server.Close()
	origin = server.URL
	client, err := New(origin, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Jobs(t.Context()); err == nil {
		t.Fatal("unauthenticated request succeeded")
	}
	client.SetCredentials("operator", "fixture")
	job, err := client.Create(t.Context(), "fixture", 10, false, project.Request{Method: "GET", URL: "https://example.test"})
	if err != nil || job.ID == "" {
		t.Fatalf("create: %+v %v", job, err)
	}
	job, err = client.SetEnabled(t.Context(), job.ID, true)
	if err != nil || !job.Enabled {
		t.Fatalf("enable: %+v %v", job, err)
	}
	jobs, err := client.Jobs(t.Context())
	if err != nil || len(jobs) != 1 || jobs[0].ID != job.ID {
		t.Fatalf("list: %+v %v", jobs, err)
	}
	if _, err := client.Executions(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := client.Delete(t.Context(), job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := client.SetEnabled(t.Context(), "../jobs", true); err == nil {
		t.Fatal("path traversal accepted")
	}
	if _, err := client.Create(t.Context(), "secret", 10, false, project.Request{Method: "GET", URL: "https://example.test", Headers: map[string]string{"Authorization": "Bearer secret"}}); err == nil {
		t.Fatal("inline credentials accepted")
	}
}

func TestClientRejectsUnsafeOriginsRedirectsAndErrorText(t *testing.T) {
	for _, origin := range []string{"http://remote.test", "https://user:secret@host.test", "https://host.test/path", "https://host.test?token=secret", "https://host.test#secret"} {
		if _, err := New(origin, nil); err == nil {
			t.Fatalf("unsafe origin accepted: %s", origin)
		}
	}
	for _, status := range []int{302, 500} {
		hits := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits++
			w.Header().Set("Location", "/redirected")
			w.WriteHeader(status)
			w.Write([]byte("secret diagnostic"))
		}))
		client, err := New(server.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.Jobs(t.Context())
		server.Close()
		if err == nil || strings.Contains(err.Error(), "secret") || hits != 1 {
			t.Fatalf("unsafe response handling: %v hits=%d", err, hits)
		}
	}
}
