package agentclient

import (
	"context"
	"slices"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

type httpScriptsKey struct{}
type httpScripts struct{ pre, post []string }

func httpScriptsFromContext(ctx context.Context) httpScripts {
	scripts, _ := ctx.Value(httpScriptsKey{}).(httpScripts)
	return scripts
}

func (client *Client) RunHTTPWithScripts(ctx context.Context, target string, payload execution.HTTPPayload,
	pre, post []string, variables map[string]string, publish func(Response),
) (Response, error) {
	if err := (project.Request{PreScripts: pre, PostScripts: post}).ValidateScripts(); err != nil {
		return Response{}, err
	}
	ctx = context.WithValue(ctx, httpScriptsKey{}, httpScripts{slices.Clone(pre), slices.Clone(post)})
	return client.runHTTPExecution(ctx, target, "http", map[string]any{
		"type": "http", "literal": true, "method": payload.Method, "headers": append([][2]string{}, payload.Headers...),
		"body": payload.Body, "followRedirects": payload.FollowRedirects,
		"bodyEncoding": payload.BodyEncoding, "multipart": payload.Multipart,
	}, variables, payload.TimeoutMS, publish)
}
