package scheduleclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"protovoy.dev/protovoy/internal/project"
	"protovoy.dev/protovoy/internal/scheduler"
)

type Client struct {
	origin         string
	http           http.Client
	user, password string
}

func New(origin string, transport http.RoundTripper) (*Client, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(origin, "#") || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("enter the schedule server origin")
	}
	ip, _ := netip.ParseAddr(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && ip.IsLoopback()) {
		return nil, errors.New("schedule server requires HTTPS or loopback HTTP")
	}
	return &Client{origin: strings.TrimRight(u.String(), "/"), http: http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// SetCredentials configures in-memory hosted Basic login for native clients.
// Browser clients use the same-origin browser login instead.
func (c *Client) SetCredentials(user, password string) { c.user, c.password = user, password }

func (c *Client) Jobs(ctx context.Context) ([]scheduler.Job, error) {
	var jobs []scheduler.Job
	err := c.request(ctx, "GET", "/v1/jobs", nil, &jobs)
	return jobs, err
}

func (c *Client) Create(ctx context.Context, name string, interval int, enabled bool, request project.Request) (scheduler.Job, error) {
	if _, err := project.CaptureHistoryRequest(request); err != nil {
		return scheduler.Job{}, err
	}
	input := struct {
		Name     string          `json:"name"`
		Interval int             `json:"intervalSeconds"`
		Enabled  bool            `json:"enabled"`
		Request  project.Request `json:"request"`
	}{name, interval, enabled, request}
	var job scheduler.Job
	err := c.request(ctx, "POST", "/v1/jobs", input, &job)
	return job, err
}

func (c *Client) SetEnabled(ctx context.Context, id string, enabled bool) (scheduler.Job, error) {
	if !validID(id) {
		return scheduler.Job{}, errors.New("invalid schedule ID")
	}
	var job scheduler.Job
	err := c.request(ctx, "PATCH", "/v1/jobs/"+id, map[string]bool{"enabled": enabled}, &job)
	return job, err
}

func (c *Client) Delete(ctx context.Context, id string) error {
	if !validID(id) {
		return errors.New("invalid schedule ID")
	}
	return c.request(ctx, "DELETE", "/v1/jobs/"+id, nil, nil)
}

func (c *Client) Executions(ctx context.Context) ([]scheduler.Execution, error) {
	var results []scheduler.Execution
	err := c.request(ctx, "GET", "/v1/executions", nil, &results)
	return results, err
}

func validID(id string) bool {
	return id != "" && len(id) <= 128 && !strings.ContainsFunc(id, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_')
	})
}

func (c *Client) request(ctx context.Context, method, path string, input, output any) error {
	var body []byte
	if input != nil {
		var err error
		body, err = json.Marshal(input)
		if err != nil {
			return err
		}
	}
	r, err := http.NewRequestWithContext(ctx, method, c.origin+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	r.Header.Set("Origin", c.origin)
	r.Header.Set("X-ProtoVoy-Scheduler", "1")
	if input != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if c.user != "" || c.password != "" {
		r.SetBasicAuth(c.user, c.password)
	}
	response, err := c.http.Do(r)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("could not reach schedule server")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("schedule server returned HTTP %d", response.StatusCode)
	}
	if output == nil {
		return nil
	}
	const limit = 16 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || len(data) > limit || json.Unmarshal(data, output) != nil {
		return errors.New("invalid schedule server response")
	}
	return nil
}
