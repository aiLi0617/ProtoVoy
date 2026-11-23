package scheduler

import (
	"encoding/json"
	"time"
)

type Job struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Enabled         bool            `json:"enabled"`
	IntervalSeconds int             `json:"intervalSeconds"`
	NextRunAt       time.Time       `json:"nextRunAt"`
	CreatedAt       time.Time       `json:"createdAt"`
	LastExecutionID string          `json:"lastExecutionId,omitempty"`
	Request         json.RawMessage `json:"request"`
}

// Execution retains outcomes without response bodies, headers or error text.
type Execution struct {
	ID         string    `json:"id"`
	JobID      string    `json:"jobId"`
	StartedAt  time.Time `json:"startedAt"`
	DurationMS int64     `json:"durationMs"`
	Success    bool      `json:"success"`
	Status     int       `json:"status,omitempty"`
}
