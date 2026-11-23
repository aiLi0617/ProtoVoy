package teamclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"protovoy.dev/protovoy/internal/execution"
)

type WorkspaceEvent struct {
	WorkspaceID string `json:"workspaceId"`
	Revision    int64  `json:"revision"`
	CommentID   string `json:"commentId"`
}

func (c *Client) Watch(ctx context.Context, connected func(), changed func(WorkspaceEvent)) error {
	path, err := c.organizationPath()
	if err != nil {
		return err
	}
	r, err := http.NewRequestWithContext(ctx, "GET", c.base+path+"/events", nil)
	if err != nil {
		return err
	}
	r.Header.Set("Authorization", "Bearer "+c.token)
	r.Header.Set("Accept", "text/event-stream")
	res, err := c.http.Do(r)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if !execution.IsEventStream(res.StatusCode, res.Header.Get("Content-Type")) {
		return fmt.Errorf("team event stream unavailable (HTTP %d)", res.StatusCode)
	}
	decoder := execution.NewSSEDecoder(res.Body, "", 0)
	for {
		event, err := decoder.Next()
		if errors.Is(err, io.EOF) {
			return errors.New("team event stream disconnected")
		}
		if err != nil {
			return err
		}
		switch event.Type {
		case "connected":
			connected()
		case "workspace.changed", "comment.changed":
			var value WorkspaceEvent
			if json.Unmarshal([]byte(event.Data), &value) != nil || !validID(value.WorkspaceID) ||
				(event.Type == "workspace.changed" && value.Revision <= 0) ||
				(event.Type == "comment.changed" && !validID(value.CommentID)) {
				return errors.New("invalid team workspace event")
			}
			changed(value)
		}
	}
}
