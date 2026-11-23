//go:build linux || darwin

package main

import (
	"context"
	"encoding/xml"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"protovoy.dev/protovoy/internal/project"
)

func TestCollectionSIGTERM(t *testing.T) {
	const childEnv = "PROTOVOY_TEST_COLLECTION_SIGTERM"
	if os.Getenv(childEnv) == "1" {
		runCollectionCommand(os.Args[3:])
		return
	}

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	started := make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		select {
		case <-r.Context().Done():
		case <-ctx.Done():
		}
	}))
	defer upstream.Close()

	dir := t.TempDir()
	projectPath := filepath.Join(dir, "project.json")
	reportPath := filepath.Join(dir, "report.xml")
	data, err := project.Encode(project.Project{
		Name:     "signal fixture",
		Requests: []project.Request{{Name: "fixture", Method: http.MethodGet, URL: upstream.URL}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCollectionSIGTERM$", "--", "--junit", reportPath, projectPath)
	cmd.Env = append(os.Environ(), childEnv+"=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	select {
	case <-started:
	case err := <-exited:
		t.Fatalf("collection exited before making the request: %v", err)
	case <-ctx.Done():
		<-exited
		t.Fatal("collection did not reach the upstream server")
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		<-exited
		t.Fatal(err)
	}
	err = <-exited
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("expected collection exit code 1 after SIGTERM, got %v", err)
	}
	data, err = os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		XMLName xml.Name `xml:"testsuite"`
		Error   string   `xml:"testcase>error"`
	}
	if err := xml.Unmarshal(data, &report); err != nil || report.Error == "" {
		t.Fatalf("expected a JUnit execution error, got %s (%v)", data, err)
	}
}
