package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/secrets"
)

func (envelope requestEnvelope) prepareRedis(useKeychain ...bool) (execution.RedisPayload, execution.RedisOptions, error) {
	var input struct {
		Type  string `json:"type"`
		Value struct {
			Commands    json.RawMessage `json:"commands"`
			Username    string          `json:"username"`
			Password    string          `json:"password"`
			PasswordRef string          `json:"passwordRef"`
			Database    json.RawMessage `json:"database"`
			Protocol    int             `json:"protocol"`
			TLS         bool            `json:"tls"`
			ServerName  string          `json:"serverName"`
			CAPEM       string          `json:"caPem"`
			Literal     bool            `json:"literal"`
		} `json:"value"`
	}
	if !decodeExecutionPayload(envelope.Payload, &input) || input.Type != "raw" {
		return execution.RedisPayload{}, execution.RedisOptions{}, errors.New("invalid Redis execution payload")
	}
	value := input.Value
	commands, err := execution.ParseRedisCommands(string(value.Commands))
	if err != nil {
		return execution.RedisPayload{}, execution.RedisOptions{}, err
	}
	payload := execution.RedisPayload{ExpectedTarget: envelope.Target, Commands: commands, Protocol: value.Protocol}
	if !value.Literal {
		if err := payload.ResolveVariables(envelope.Variables); err != nil {
			return execution.RedisPayload{}, execution.RedisOptions{}, err
		}
	}
	target, secure, err := redisTarget(payload.ExpectedTarget)
	if err != nil {
		return execution.RedisPayload{}, execution.RedisOptions{}, err
	}
	payload.ExpectedTarget = target
	database, err := redisDatabase(value.Database)
	if err != nil {
		return execution.RedisPayload{}, execution.RedisOptions{}, err
	}
	options := execution.RedisOptions{
		Protocol: value.Protocol, Username: value.Username, Password: value.Password, Database: database,
		PasswordRef: value.PasswordRef,
		TLS:         value.TLS || secure, ServerName: value.ServerName, CAPEM: value.CAPEM,
	}
	resolve := func(string) (string, error) { return os.Getenv(execution.RedisPasswordEnv), nil }
	if len(useKeychain) > 0 && useKeychain[0] {
		resolve = secrets.Resolve
	}
	options, err = execution.ResolveRedisOptions(target, options, resolve)
	return payload, options, err
}

func redisTarget(target string) (string, bool, error) {
	invalid := errors.New("enter a Redis host:port, redis:// or rediss:// endpoint")
	secure := false
	if strings.Contains(target, "://") {
		parsed, err := url.Parse(target)
		if err != nil || (parsed.Scheme != "redis" && parsed.Scheme != "rediss") || parsed.User != nil ||
			(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.ForceQuery || strings.Contains(target, "#") {
			return "", false, invalid
		}
		target, secure = parsed.Host, parsed.Scheme == "rediss"
	}
	if strings.ContainsAny(target, "@/\r\n\x00 \t?#") || target == "" {
		return "", false, invalid
	}
	if !strings.Contains(target, ":") || strings.HasPrefix(target, "[") && strings.HasSuffix(target, "]") {
		target += ":6379"
	}
	host, port, err := net.SplitHostPort(target)
	if err != nil || host == "" || port == "" {
		return "", false, invalid
	}
	return target, secure, nil
}

func redisDatabase(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var database string
	if json.Unmarshal(raw, &database) != nil {
		database = string(raw)
	}
	if database != "" {
		if _, err := strconv.ParseUint(database, 10, 64); err != nil {
			return "", errors.New("Redis database must be a nonnegative integer")
		}
	}
	return database, nil
}

func (job *agentJob) executeRedis(ctx context.Context, id, requestID string, payload execution.RedisPayload, options execution.RedisOptions, timeout time.Duration) agentEvent {
	started := time.Now()
	if !job.emit(ctx, agentEvent{"type": "state_changed", "state": "running", "phase": "connect"}) {
		return terminalFailure(ctx, "connection")
	}
	values, err := execution.ExecuteRedisWithOptions(ctx, payload.ExpectedTarget, payload.Commands, timeout, options)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return agentEvent{"type": "failed", "code": "timeout", "message": "request timed out"}
		}
		return terminalFailure(ctx, "transfer")
	}
	data, err := json.Marshal(values)
	if err != nil || len(data) > execution.RedisJSONResponseLimit {
		return agentEvent{"type": "failed", "code": "transfer", "message": "could not encode Redis response within its limit"}
	}
	const contentType = "application/json"
	if !job.emit(ctx, agentEvent{"type": "response_meta", "status": nil, "contentType": contentType, "headers": [][2]string{}}) ||
		!job.emitBinary(ctx, contentType, data) {
		return terminalFailure(ctx, "transfer")
	}
	final := agentEvent{"type": "response_chunk", "content_type": contentType, "size": 0, "done": true}
	if len(data) <= execution.PreviewLimit {
		final["preview"] = string(data)
	}
	if !job.emit(ctx, final) {
		return terminalFailure(ctx, "transfer")
	}
	return completedEvent(id, requestID, "redis", 0, int64(len(data)), started)
}
