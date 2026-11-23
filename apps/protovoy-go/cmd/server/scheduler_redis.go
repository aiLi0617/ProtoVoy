package main

import (
	"context"
	"errors"
	"strconv"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
	"protovoy.dev/protovoy/internal/scheduler"
)

func scheduledRedisExecutor(target string, options execution.RedisOptions, reference string) scheduler.Executor {
	return func(ctx context.Context, request project.Request) (scheduler.Outcome, error) {
		if target == "" || request.Redis == nil || request.HasScripts() || !execution.ValidRequestTimeout(request.BrokerTimeoutMS) {
			return scheduler.Outcome{}, errors.New("scheduled Redis endpoint is not available for this request")
		}
		settings := request.Redis
		commands, err := execution.ParseRedisCommands(request.Body)
		if err != nil {
			return scheduler.Outcome{}, err
		}
		input := execution.RedisPayload{ExpectedTarget: request.URL, Commands: commands}
		if err = input.ResolveVariables(request.Variables); err != nil {
			return scheduler.Outcome{}, err
		}
		if input.ExpectedTarget != target || (settings.Protocol != 2 && settings.Protocol != 3) {
			return scheduler.Outcome{}, errors.New("scheduled Redis target or protocol does not match configuration")
		}
		if err = validateScheduledRedisBinding(*settings, options, reference); err != nil {
			return scheduler.Outcome{}, err
		}
		configured := options
		configured.Protocol = settings.Protocol
		values, err := execution.ExecuteRedisWithOptions(ctx, target, input.Commands, execution.RequestTimeout(request.BrokerTimeoutMS), configured)
		if err != nil {
			return scheduler.Outcome{}, err
		}
		for _, value := range values {
			if value.Kind == "error" {
				return scheduler.Outcome{}, errors.New("Redis command returned an error")
			}
		}
		return scheduler.Outcome{Success: true}, nil
	}
}

func validateScheduledRedisBinding(settings project.RedisSettings, options execution.RedisOptions, reference string) error {
	if settings.PasswordRef == "" {
		if settings.Username != "" || settings.Database != "" || settings.ServerName != "" {
			return errors.New("Redis credential assertions require a named binding")
		}
		return nil
	}
	if !execution.ValidPasswordReference(settings.PasswordRef) || settings.PasswordRef != reference || !options.TLS || options.Password == "" {
		return errors.New("Redis password reference does not match configuration")
	}
	if settings.Username != "" && settings.Username != options.Username || settings.ServerName != "" && settings.ServerName != options.ServerName {
		return errors.New("Redis credential settings mismatch")
	}
	if settings.Database != "" {
		configured := options.Database
		if configured == "" {
			configured = "0"
		}
		actual, err := strconv.ParseUint(configured, 10, 64)
		expected, expectedErr := strconv.ParseUint(settings.Database, 10, 64)
		if err != nil || expectedErr != nil || actual != expected {
			return errors.New("Redis database mismatch")
		}
	}
	return nil
}
