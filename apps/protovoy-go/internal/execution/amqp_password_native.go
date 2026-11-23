//go:build !js

package execution

import (
	"errors"
	"os"
	"strings"
)

// Password references pin the full URI, including its virtual host, and TLS trust.
func resolveAMQPPassword(target string, options *AMQPOptions, resolve func(string) (string, error)) error {
	if options.PasswordRef == "" {
		return nil
	}
	username, serverName := os.Getenv("PROTOVOY_AMQP_USERNAME"), os.Getenv("PROTOVOY_AMQP_SERVER_NAME")
	if !ValidPasswordReference(options.PasswordRef) || options.PasswordRef != os.Getenv(AMQPPasswordRefEnv) ||
		target != os.Getenv("PROTOVOY_AMQP_TARGET") || !strings.HasPrefix(target, "amqps://") || username == "" ||
		options.Password != "" || options.CAPEM != "" || options.Username != "" && options.Username != username ||
		options.ServerName != "" && options.ServerName != serverName {
		return errors.New("AMQP password reference is unavailable for these connection settings")
	}
	if path := os.Getenv("PROTOVOY_AMQP_CA_FILE"); path != "" {
		data, err := ReadCAFile(path)
		if err != nil || len(data) == 0 {
			return errors.New("could not load the configured AMQP CA")
		}
		options.CAPEM = string(data)
	}
	if _, err := NewAMQPBroker(target, username, "", serverName, options.CAPEM); err != nil {
		return errors.New("AMQP password reference requires a valid configured TLS endpoint")
	}
	if resolve == nil {
		return errors.New("AMQP password reference could not be resolved")
	}
	password, err := resolve(options.PasswordRef)
	if err != nil || password == "" || len(username)+len(password) > 2048 || strings.ContainsRune(password, 0) {
		return errors.New("AMQP password reference could not be resolved")
	}
	options.Username, options.Password, options.ServerName, options.PasswordRef = username, password, serverName, ""
	return nil
}
