//go:build !js

package scheduler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestScheduleHTTPManagementAndAuthorization(t *testing.T) {
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h := s.Handler(func(r *http.Request) bool { return r.Header.Get("Authorization") == "Bearer fixture" })
	call := func(method, path, body string, authenticated bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if authenticated {
			r.Header.Set("Authorization", "Bearer fixture")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/v1/jobs", "/v1/executions"} {
		if w := call("GET", path, "", false); w.Code != 403 {
			t.Fatal(w.Code)
		}
	}
	valid := `{"name":"scheduled","intervalSeconds":10,"enabled":true,"request":{"url":"https://example.test","method":"GET"}}`
	if w := call("POST", "/v1/jobs", valid, false); w.Code != 403 {
		t.Fatal(w.Code)
	}
	for _, body := range []string{valid + ` {}`, strings.Replace(valid, `"intervalSeconds":10`, `"intervalSeconds":1`, 1), strings.Replace(valid, `"method":"GET"`, `"method":"GET","headers":{"Authorization":"Bearer secret"}`, 1), `{"unexpected":true}`, strings.Repeat("x", 385<<10)} {
		if w := call("POST", "/v1/jobs", body, true); w.Code != 400 {
			t.Fatalf("invalid input returned %d", w.Code)
		}
	}
	w := call("POST", "/v1/jobs", valid, true)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var job Job
	if err := json.Unmarshal(w.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	if w := call("PATCH", "/v1/jobs/"+job.ID, `{}`, true); w.Code != 400 {
		t.Fatal(w.Code)
	}
	w = call("PATCH", "/v1/jobs/"+job.ID, `{"enabled":false}`, true)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &job); err != nil || job.Enabled {
		t.Fatal("disable failed", err)
	}
	if w := call("GET", "/v1/jobs", "", true); w.Code != 200 || !strings.Contains(w.Body.String(), job.ID) {
		t.Fatal("missing schedule")
	}
	if w := call("DELETE", "/v1/jobs/"+job.ID, "", true); w.Code != 204 {
		t.Fatal(w.Code)
	}
	if w := call("PATCH", "/v1/jobs/"+job.ID, `{"enabled":true}`, true); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := call("DELETE", "/v1/jobs/"+job.ID, "", true); w.Code != 404 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	s.Handler(nil).ServeHTTP(w, httptest.NewRequest("GET", "/v1/jobs", nil))
	if w.Code != 403 {
		t.Fatal("missing authorizer allowed access")
	}
}
