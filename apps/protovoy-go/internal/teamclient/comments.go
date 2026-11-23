package teamclient

import (
	"context"
	"errors"
	"net/url"
	"time"
)

type Comment struct {
	ID         string     `json:"id"`
	ActorID    string     `json:"actorId"`
	ActorName  string     `json:"actorName"`
	ParentID   *string    `json:"parentId"`
	Body       string     `json:"body"`
	ResolvedAt *time.Time `json:"resolvedAt"`
}

func (c *Client) Comments(ctx context.Context, workspace string) ([]Comment, error) {
	path, err := c.workspacePath(workspace)
	if err != nil {
		return nil, err
	}
	comments := []Comment{}
	err = c.request(ctx, "GET", path+"/comments", nil, &comments)
	return comments, err
}

func (c *Client) PostComment(ctx context.Context, workspace, parent, body string) (Comment, error) {
	input := map[string]string{"body": body}
	if parent != "" {
		if !validID(parent) {
			return Comment{}, errors.New("invalid parent comment ID")
		}
		input["parentId"] = parent
	}
	return c.changeComment(ctx, workspace, "", "", "POST", input)
}

func (c *Client) EditComment(ctx context.Context, workspace, id, body string) (Comment, error) {
	return c.changeComment(ctx, workspace, id, "", "PATCH", map[string]string{"body": body})
}

func (c *Client) ResolveComment(ctx context.Context, workspace, id string, resolved bool) (Comment, error) {
	return c.changeComment(ctx, workspace, id, "/resolution", "PATCH", map[string]bool{"resolved": resolved})
}

func (c *Client) changeComment(ctx context.Context, workspace, id, suffix, method string, input any) (Comment, error) {
	path, err := c.workspacePath(workspace)
	if err != nil {
		return Comment{}, err
	}
	path += "/comments"
	if method == "PATCH" {
		if !validID(id) {
			return Comment{}, errors.New("invalid comment ID")
		}
		path += "/" + url.PathEscape(id) + suffix
	}
	var comment Comment
	err = c.request(ctx, method, path, input, &comment)
	return comment, err
}
