package agentclient

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"protovoy.dev/protovoy/internal/execution"
)

func (client *Client) RunRedis(ctx context.Context, payload execution.RedisPayload, options execution.RedisOptions) ([]execution.RedisValue, error) {
	if !execution.ValidRequestTimeout(payload.TimeoutMS) {
		return nil, errors.New("invalid Redis execution timeout")
	}
	timeout := execution.RequestTimeout(payload.TimeoutMS)
	ctx, cancel := context.WithTimeout(ctx, timeout+5*time.Second)
	defer cancel()
	stream, err := client.openExecutionWithVariables(ctx, payload.ExpectedTarget, "redis", map[string]any{
		"type": "raw", "value": map[string]any{
			"commands": payload.Commands, "protocol": payload.Protocol, "literal": true,
			"username": options.Username, "password": options.Password, "passwordRef": options.PasswordRef, "database": options.Database,
			"tls": options.TLS, "serverName": options.ServerName, "caPem": options.CAPEM,
		},
	}, nil, int(timeout/time.Millisecond))
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	data, err := stream.readBinary(execution.RedisJSONResponseLimit, "Redis")
	if err != nil {
		return nil, err
	}
	var values []execution.RedisValue
	if json.Unmarshal(data, &values) != nil || len(values) != len(payload.Commands) {
		return nil, errors.New("Local Agent returned an invalid Redis response")
	}
	return values, nil
}
