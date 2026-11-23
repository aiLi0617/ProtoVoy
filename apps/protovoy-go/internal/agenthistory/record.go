package agenthistory

import "time"

const MaxResponseBytes = 1 << 20

// Record is the metadata the Agent retains after a request ends.
type Record struct {
	ID                    string    `json:"id"`
	RequestID             string    `json:"requestId"`
	SnapshotID            string    `json:"snapshotId,omitempty"`
	ProtocolID            string    `json:"protocolId"`
	State                 string    `json:"state"`
	Status                *int      `json:"status"`
	DurationMS            int64     `json:"durationMs"`
	BytesReceived         int64     `json:"bytesReceived"`
	StartedAt             time.Time `json:"startedAt"`
	FinishedAt            time.Time `json:"finishedAt"`
	ResponseBodyBytes     *int      `json:"responseBodyBytes,omitempty"`
	ResponseBodyTruncated bool      `json:"responseBodyTruncated,omitempty"`
	ResponseBodyText      bool      `json:"responseBodyText,omitempty"`
}
