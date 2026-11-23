package agentclient

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	"protovoy.dev/protovoy/internal/agenthistory"
	"protovoy.dev/protovoy/internal/execution"
)

func (client *Client) History(ctx context.Context) ([]agenthistory.Record, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var records []agenthistory.Record
	err := client.json(ctx, http.MethodGet, "/v1/history?limit=30", nil, &records)
	return records, err
}

type HistoryResponse struct {
	Data      []byte
	Text      bool
	Truncated bool
}

func (client *Client) HistoryResponse(ctx context.Context, id string) (HistoryResponse, error) {
	if !validExecutionID(id) {
		return HistoryResponse{}, errors.New("invalid execution ID")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := client.request(ctx, http.MethodGet, "/v1/history/"+id+"/body", "*/*", nil)
	if err != nil {
		return HistoryResponse{}, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, execution.PreviewLimit+1))
	if err != nil {
		return HistoryResponse{}, errors.New("could not read captured response")
	}
	media, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	return HistoryResponse{
		Data: data[:min(len(data), execution.PreviewLimit)], Text: media == "text/plain", Truncated: len(data) > execution.PreviewLimit,
	}, nil
}
