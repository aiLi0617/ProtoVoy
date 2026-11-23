package agentclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"protovoy.dev/protovoy/internal/project"
)

var (
	ErrNotFound = errors.New("Local Agent resource was not found")
	ErrConflict = errors.New("Local Agent resource changed; reload before saving")
)

func (client *Client) SupportsWorkspace() bool {
	return client.workspaceVersion == "1"
}

func (client *Client) Workspace(ctx context.Context) (project.Workspace, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := client.request(ctx, http.MethodGet, "/v1/workspace", "application/json", nil)
	if err != nil {
		return project.Workspace{}, "", err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, project.MaxBytes+1))
	if err != nil {
		return project.Workspace{}, "", errors.New("could not read Agent workspace")
	}
	value, err := project.DecodeWorkspace(data)
	return value, response.Header.Get("ETag"), err
}

func (client *Client) SaveWorkspace(ctx context.Context, value project.Workspace, etag string) error {
	data, err := project.EncodeWorkspace(value)
	if err != nil {
		return err
	}
	return client.changeWorkspace(ctx, http.MethodPut, data, etag)
}

func (client *Client) CreateWorkspace(ctx context.Context, value project.Workspace) error {
	data, err := project.EncodeWorkspace(value)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := client.requestWithHeaders(ctx, http.MethodPut, "/v1/workspace", "application/json", data, http.Header{"If-None-Match": {"*"}})
	if err != nil {
		return err
	}
	return response.Body.Close()
}

func (client *Client) DeleteWorkspace(ctx context.Context, etag string) error {
	return client.changeWorkspace(ctx, http.MethodDelete, nil, etag)
}

func (client *Client) changeWorkspace(ctx context.Context, method string, data []byte, etag string) error {
	if etag == "" {
		return errors.New("read the Agent workspace version before changing it")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := client.requestWithHeaders(ctx, method, "/v1/workspace", "application/json", data, http.Header{"If-Match": {etag}})
	if err != nil {
		return err
	}
	return response.Body.Close()
}
