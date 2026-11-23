package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/secrets"
)

func protocolPassword(protocol, reference string) (string, error) {
	names := map[string]string{
		"sql": execution.SQLPasswordEnv, "redis": execution.RedisPasswordEnv,
		"mqtt": execution.MQTTPasswordEnv, "amqp": execution.AMQPPasswordEnv,
		"kafka": execution.KafkaPasswordEnv,
	}
	name := names[protocol]
	if reference == "" {
		return os.Getenv(name), nil
	}
	if name == "" {
		return "", errors.New("-password-secret requires sql, redis, mqtt, amqp, or kafka")
	}
	value, err := secrets.Resolve(reference)
	if err != nil {
		return "", errors.New("could not resolve protocol password secret")
	}
	return value, nil
}

func addSecretHeaders(headers, references []string) ([]string, error) {
	result := append([]string(nil), headers...)
	for _, item := range references {
		name, reference, ok := strings.Cut(item, ":")
		if !ok || strings.ContainsAny(item, "\r\n") {
			return nil, errors.New("-header-secret requires Name: secret-reference")
		}
		value, err := secrets.Resolve(strings.TrimSpace(reference))
		if err != nil {
			return nil, errors.New("could not resolve request header secret")
		}
		line := strings.TrimSpace(name) + ": " + value
		if _, err := parseCLIHeaders([]string{line}); err != nil {
			return nil, errors.New("secret cannot be used as the requested header")
		}
		result = append(result, line)
	}
	return result, nil
}

func runSecretCommand(args []string, input io.Reader, output io.Writer) error {
	usage := errors.New("usage: protovoy secret set|exists|delete|exists-http|delete-http REFERENCE, bind-http REFERENCE URL HEADER, or bind-oauth-http REFERENCE URL")
	if len(args) != 2 && !(len(args) == 4 && args[0] == "bind-http") && !(len(args) == 3 && args[0] == "bind-oauth-http") {
		return usage
	}
	switch args[0] {
	case "set", "bind-http", "bind-oauth-http":
		if args[0] == "bind-http" && len(args) != 4 {
			return usage
		}
		if args[0] == "bind-oauth-http" {
			if len(args) != 3 {
				return usage
			}
			var settings execution.OAuthTokenRequest
			data, err := io.ReadAll(io.LimitReader(input, 2561))
			if err != nil || len(data) > 2560 {
				return errors.New("OAuth binding input exceeds 2560 bytes or could not be read")
			}
			decoder := json.NewDecoder(bytes.NewReader(data))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&settings) != nil || decoder.Decode(new(any)) != io.EOF {
				return errors.New("enter a bounded OAuth binding JSON document on standard input")
			}
			if err := secrets.PutHTTPOAuth(args[1], args[2], settings); err != nil {
				return err
			}
			_, err = fmt.Fprintln(output, "OAuth HTTP binding saved")
			return err
		}
		data, err := io.ReadAll(io.LimitReader(input, 2563))
		if err != nil {
			return errors.New("could not read secret from standard input")
		}
		value := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
		if args[0] == "bind-http" {
			err = secrets.PutHTTP(args[1], args[2], args[3], value)
		} else {
			err = secrets.Put(args[1], value)
		}
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(output, "Secret saved")
		return err
	case "exists":
		_, err := secrets.Resolve(args[1])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(output, "Secret exists")
		return err
	case "delete":
		if err := secrets.Delete(args[1]); err != nil {
			return err
		}
		_, err := fmt.Fprintln(output, "Secret deleted")
		return err
	case "exists-http":
		exists, err := secrets.HTTPExists(args[1])
		if err != nil {
			return err
		}
		if !exists {
			return secrets.ErrNotFound
		}
		_, err = fmt.Fprintln(output, "HTTP binding exists")
		return err
	case "delete-http":
		if err := secrets.DeleteHTTP(args[1]); err != nil {
			return err
		}
		_, err := fmt.Fprintln(output, "HTTP binding deleted")
		return err
	default:
		return usage
	}
}

func secretCommand(args []string) {
	if err := runSecretCommand(args, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
