package teamclient

import (
	"context"
	"encoding/json"
	"time"
)

type AuditEvent struct {
	ActorID      string          `json:"actorId"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resourceType"`
	ResourceID   string          `json:"resourceId"`
	Details      json.RawMessage `json:"details"`
	CreatedAt    time.Time       `json:"createdAt"`
}

func (c *Client) Audit(ctx context.Context) ([]AuditEvent, error) {
	path, err := c.organizationPath()
	if err != nil {
		return nil, err
	}
	events := []AuditEvent{}
	err = c.request(ctx, "GET", path+"/audit", nil, &events)
	return events, err
}
