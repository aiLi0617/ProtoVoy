//go:build !js

package execution

import (
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
)

func redisReferenceDatabase(value string) (string, error) {
	if value == "" {
		return "0", nil
	}
	number, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return "", errors.New("invalid configured Redis database")
	}
	return strconv.FormatUint(number, 10), nil
}

// ResolveRedisOptions binds initial connection settings, not Redis command ACLs.
func ResolveRedisOptions(target string, options RedisOptions, resolve func(string) (string, error)) (RedisOptions, error) {
	if options.PasswordRef == "" {
		return options, nil
	}
	invalid := errors.New("Redis password reference is unavailable for these connection settings")
	username, serverName := os.Getenv("PROTOVOY_REDIS_USERNAME"), os.Getenv("PROTOVOY_REDIS_SERVER_NAME")
	database, err := redisReferenceDatabase(os.Getenv("PROTOVOY_REDIS_DATABASE"))
	if err != nil {
		return RedisOptions{}, invalid
	}
	if !ValidPasswordReference(options.PasswordRef) || options.PasswordRef != os.Getenv(RedisPasswordRefEnv) ||
		target != os.Getenv("PROTOVOY_REDIS_TARGET") || !options.TLS || options.Password != "" || options.CAPEM != "" ||
		options.Username != "" && options.Username != username || options.ServerName != "" && options.ServerName != serverName {
		return RedisOptions{}, invalid
	}
	if options.Database != "" {
		requested, err := redisReferenceDatabase(options.Database)
		if err != nil || requested != database {
			return RedisOptions{}, invalid
		}
	}
	host, port, err := net.SplitHostPort(target)
	parsedPort, portErr := strconv.ParseUint(port, 10, 16)
	if err != nil || portErr != nil || parsedPort == 0 || host == "" || strings.ContainsAny(target, "@/\\\r\n\x00 \t?#") {
		return RedisOptions{}, invalid
	}
	if path := os.Getenv("PROTOVOY_REDIS_CA_FILE"); path != "" {
		data, err := ReadCAFile(path)
		if err != nil || len(data) == 0 {
			return RedisOptions{}, errors.New("could not load the configured Redis CA")
		}
		options.CAPEM = string(data)
		if _, err := socketRoots(options.CAPEM, nil); err != nil {
			return RedisOptions{}, errors.New("could not load the configured Redis CA")
		}
	}
	if resolve == nil {
		return RedisOptions{}, errors.New("Redis password reference could not be resolved")
	}
	password, err := resolve(options.PasswordRef)
	if err != nil || password == "" || len(password) > redisMessageLimit {
		return RedisOptions{}, errors.New("Redis password reference could not be resolved")
	}
	options.Username, options.Password, options.ServerName, options.Database, options.PasswordRef = username, password, serverName, database, ""
	return options, nil
}
