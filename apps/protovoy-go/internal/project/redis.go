package project

import (
	"errors"
	"slices"
	"strings"

	"protovoy.dev/protovoy/internal/execution"
)

// RedisSettings holds collection settings. A connection password is supplied
// separately at runtime and has no field in the workspace format.
type RedisSettings struct {
	Protocol    int    `json:"protocol"`
	Username    string `json:"username,omitempty"`
	PasswordRef string `json:"passwordRef,omitempty"`
	Database    string `json:"database,omitempty"`
	TLS         bool   `json:"tls,omitempty"`
	ServerName  string `json:"serverName,omitempty"`
	CAPEM       string `json:"caPem,omitempty"`
}

func (settings RedisSettings) CheckPersistence(target, body string) error {
	if !settings.validProtocol() {
		return errors.New("select Redis RESP2 or RESP3")
	}
	commands, err := execution.ParseRedisCommands(body)
	if err != nil {
		return err
	}
	if strings.Contains(target, "@") || settings.hasSensitiveData(commands) || sensitiveBody.MatchString(body) {
		return errors.New("remove possible credentials from the Redis request before saving")
	}
	return nil
}

func (settings RedisSettings) validProtocol() bool {
	return execution.ValidPasswordReference(settings.PasswordRef) && (settings.Protocol == 2 || settings.Protocol == 3)
}

func (settings RedisSettings) hasSensitiveData(commands [][]string) bool {
	if hasPrivateKey(settings.CAPEM) {
		return true
	}
	for _, args := range commands {
		command := strings.ToUpper(strings.TrimSpace(args[0]))
		if strings.Contains(command, "{{") || command == "AUTH" {
			return true
		}
		if len(args) < 2 {
			continue
		}
		subcommand := strings.ToUpper(strings.TrimSpace(args[1]))
		switch command {
		case "HELLO", "MIGRATE":
			if slices.ContainsFunc(args[1:], func(arg string) bool {
				arg = strings.TrimSpace(arg)
				return strings.EqualFold(arg, "AUTH") || strings.EqualFold(arg, "AUTH2") || strings.Contains(arg, "{{")
			}) {
				return true
			}
		case "ACL", "CONFIG", "SENTINEL":
			if strings.Contains(subcommand, "{{") || subcommand == "SETUSER" || subcommand == "SET" || subcommand == "CONFIG" {
				return true
			}
		}
	}
	return false
}
