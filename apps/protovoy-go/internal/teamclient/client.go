package teamclient

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

	"protovoy.dev/protovoy/internal/project"
)

type Client struct {
	base         string
	http         http.Client
	token        string
	Organization string
	UserID       string
	Role         string
}

func New(origin string, transport http.RoundTripper) (*Client, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery ||
		strings.Contains(origin, "#") || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("enter the team server origin")
	}
	ip, _ := netip.ParseAddr(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && ip.IsLoopback()) {
		return nil, errors.New("the team server requires HTTPS or a loopback HTTP address")
	}
	return &Client{
		base: strings.TrimRight(u.String(), "/") + "/api/team/v1",
		http: http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
	}, nil
}

func (c *Client) Login(ctx context.Context, email, password string) error {
	return c.authenticate(ctx, "/auth/login", map[string]string{"email": email, "password": password})
}

func (c *Client) CompleteSSO(ctx context.Context) error {
	return c.authenticate(ctx, "/auth/sso/session", nil)
}

func (c *Client) SSOAvailable(ctx context.Context) (bool, error) {
	var config struct{ Enabled bool }
	err := c.request(ctx, "GET", "/auth/sso/config", nil, &config)
	return config.Enabled, err
}

func (c *Client) authenticate(ctx context.Context, path string, input any) error {
	var result struct {
		Token        string              `json:"token"`
		Organization string              `json:"organizationId"`
		User         struct{ ID string } `json:"user"`
		Role         string              `json:"role"`
	}
	if err := c.request(ctx, "POST", path, input, &result); err != nil {
		return err
	}
	if result.Token == "" || !validID(result.Organization) {
		return errors.New("invalid team login response")
	}
	c.token, c.Organization = result.Token, result.Organization
	c.UserID, c.Role = result.User.ID, result.Role
	return nil
}

func (c *Client) Logout(ctx context.Context) error {
	if c.token == "" {
		return nil
	}
	if err := c.request(ctx, "POST", "/auth/logout", nil, nil); err != nil {
		return err
	}
	c.token, c.Organization = "", ""
	c.UserID, c.Role = "", ""
	return nil
}

type snapshot struct {
	Revision int64           `json:"revision"`
	Document json.RawMessage `json:"document"`
}

type Conflict struct {
	Revision int64           `json:"currentRevision"`
	Document json.RawMessage `json:"currentDocument"`
}

func (*Conflict) Error() string { return "team workspace revision conflict" }

func (c *Client) Load(ctx context.Context, id string) (project.Project, int64, error) {
	path, err := c.workspacePath(id)
	if err != nil {
		return project.Project{}, 0, err
	}
	var value snapshot
	if err := c.request(ctx, "GET", path, nil, &value); err != nil {
		return project.Project{}, 0, err
	}
	if value.Revision == 0 && bytes.Equal(value.Document, []byte("{}")) {
		return project.Project{}, 0, nil
	}
	if value.Revision <= 0 {
		return project.Project{}, 0, errors.New("invalid team workspace revision")
	}
	collection, err := project.Read(bytes.NewReader(value.Document))
	if err != nil {
		return project.Project{}, 0, err
	}
	return collection, value.Revision, nil
}

func (c *Client) Save(ctx context.Context, id string, base int64, collection project.Project) (int64, error) {
	path, err := c.workspacePath(id)
	if err != nil {
		return 0, err
	}
	if base < 0 {
		return 0, errors.New("invalid base revision")
	}
	document, err := project.Encode(collection)
	if err != nil {
		return 0, err
	}
	input := struct {
		Base     int64           `json:"baseRevision"`
		Document json.RawMessage `json:"document"`
	}{base, document}
	var value snapshot
	if err := c.request(ctx, "PUT", path, input, &value); err != nil {
		return 0, err
	}
	if value.Revision <= base {
		return 0, errors.New("invalid team workspace revision")
	}
	return value.Revision, nil
}

func (c *Client) workspacePath(id string) (string, error) {
	if c.token == "" || !validID(c.Organization) {
		return "", errors.New("sign in to the team server first")
	}
	if !validID(id) {
		return "", errors.New("invalid team workspace ID")
	}
	return "/organizations/" + url.PathEscape(c.Organization) + "/workspaces/" + url.PathEscape(id), nil
}

func validID(id string) bool {
	return id != "" && len(id) <= 128 && id != "." && id != ".." && !strings.ContainsAny(id, "/\\")
}

func (c *Client) request(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		if len(data) > project.MaxBytes {
			return errors.New("team workspace update exceeds 8 MiB")
		}
		body = bytes.NewReader(data)
	}
	r, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" && path != "/auth/login" {
		r.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.http.Do(r)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent && output == nil {
		return nil
	}
	conflicted := method == "PUT" && response.StatusCode == http.StatusConflict
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated && !conflicted {
		return fmt.Errorf("team server returned HTTP %d", response.StatusCode)
	}
	const maxResponse = project.MaxBytes + 4096
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if err != nil || len(data) > maxResponse {
		return errors.New("could not read team server response within size limit")
	}
	if conflicted {
		var conflict Conflict
		if json.Unmarshal(data, &conflict) != nil || conflict.Document == nil || conflict.Revision < 0 {
			return errors.New("invalid team conflict response")
		}
		return &conflict
	}
	if output == nil || json.Unmarshal(data, output) != nil {
		return errors.New("invalid team server response")
	}
	return nil
}
