package agentclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"protovoy.dev/protovoy/internal/mock"
)

type MockStatus struct {
	Running bool   `json:"running"`
	Address string `json:"address"`
}

func (client *Client) SupportsMock() bool {
	return client.mockVersion == "1"
}

func (client *Client) MockStatus(ctx context.Context) (MockStatus, error) {
	return client.mockRequest(ctx, http.MethodGet, nil)
}

func (client *Client) StartMock(ctx context.Context, data []byte) (MockStatus, error) {
	rules, err := mock.Parse(data)
	if err != nil {
		return MockStatus{}, err
	}
	data, err = mock.Encode(rules)
	if err != nil {
		return MockStatus{}, err
	}
	return client.mockRequest(ctx, http.MethodPost, data)
}

func (client *Client) StopMock(ctx context.Context) (MockStatus, error) {
	return client.mockRequest(ctx, http.MethodDelete, nil)
}

func (client *Client) mockRequest(ctx context.Context, method string, data []byte) (MockStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	var status MockStatus
	response, err := client.request(ctx, method, "/v1/mock", "application/json", data)
	if err != nil {
		return status, err
	}
	defer response.Body.Close()
	err = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&status)
	return status, err
}
