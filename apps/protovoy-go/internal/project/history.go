package project

import (
	"encoding/json"
	"errors"
)

const MaxHistorySnapshots = 20
const MaxHistorySnapshotBytes = 256 << 10
const MaxHistorySnapshotTotal = 1 << 20

type HistorySnapshot struct {
	ID      string          `json:"id"`
	Request json.RawMessage `json:"request"`
}

func CaptureHistoryRequest(request Request) (json.RawMessage, error) {
	request.LocalID = ""
	if request.containsSensitiveData() {
		return nil, errors.New("request contains possible credentials")
	}
	if err := validatePortableRequest(&request, true); err != nil {
		return nil, err
	}
	data, err := json.Marshal(request)
	if err != nil || len(data) > MaxHistorySnapshotBytes {
		return nil, errors.New("request snapshot exceeds 256 KiB")
	}
	return data, nil
}

func OpenHistoryRequest(data json.RawMessage) (Request, error) {
	var request Request
	if len(data) > MaxHistorySnapshotBytes || !decodeLegacy(data, &request) {
		return Request{}, errors.New("invalid request snapshot")
	}
	if request.LocalID != "" || request.containsSensitiveData() {
		return Request{}, errors.New("request snapshot contains local identity or possible credentials")
	}
	if err := validatePortableRequest(&request, true); err != nil {
		return Request{}, err
	}
	return request, nil
}
