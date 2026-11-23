package teamclient

import (
	"context"
	"errors"
	"net/url"
)

type Member struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
}

func (c *Client) organizationPath() (string, error) {
	if c.token == "" || !validID(c.Organization) {
		return "", errors.New("sign in to the team server first")
	}
	return "/organizations/" + url.PathEscape(c.Organization), nil
}

func (c *Client) Members(ctx context.Context) ([]Member, error) {
	path, err := c.organizationPath()
	if err != nil {
		return nil, err
	}
	var members []Member
	err = c.request(ctx, "GET", path+"/members", nil, &members)
	return members, err
}

func (c *Client) SaveMember(ctx context.Context, email, displayName, password, role string, create bool) (Member, error) {
	path, err := c.organizationPath()
	if err != nil {
		return Member{}, err
	}
	path += "/members"
	input := map[string]string{"email": email, "role": role}
	if create {
		path += "/provision"
		input["displayName"], input["password"] = displayName, password
	}
	var member Member
	err = c.request(ctx, "POST", path, input, &member)
	return member, err
}
