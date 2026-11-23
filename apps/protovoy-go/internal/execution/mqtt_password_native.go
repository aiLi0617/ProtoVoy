//go:build !js

package execution

import (
	"errors"
	"os"
	"strings"
)

func resolveMQTTPassword(target string, options *MQTTOptions, resolve func(string) (string, error)) error {
	if options.PasswordRef == "" {
		return nil
	}
	username, serverName := os.Getenv("PROTOVOY_MQTT_USERNAME"), os.Getenv("PROTOVOY_MQTT_SERVER_NAME")
	if !ValidPasswordReference(options.PasswordRef) || options.PasswordRef != os.Getenv(MQTTPasswordRefEnv) ||
		target != os.Getenv("PROTOVOY_MQTT_TARGET") || !strings.HasPrefix(target, "mqtts://") || username == "" ||
		options.Password != "" || options.CAPEM != "" || (options.Username != "" && options.Username != username) ||
		(options.ServerName != "" && options.ServerName != serverName) {
		return errors.New("MQTT password reference is unavailable for these connection settings")
	}
	if _, err := NewMQTTBroker(target, serverName, ""); err != nil {
		return errors.New("MQTT password reference requires a valid configured TLS endpoint")
	}
	if path := os.Getenv("PROTOVOY_MQTT_CA_FILE"); path != "" {
		data, err := ReadCAFile(path)
		if err != nil || len(data) == 0 {
			return errors.New("could not load the configured MQTT CA")
		}
		options.CAPEM = string(data)
	}
	if resolve == nil {
		return errors.New("MQTT password reference could not be resolved")
	}
	password, err := resolve(options.PasswordRef)
	if err != nil || password == "" || len(password) > 65535 {
		return errors.New("MQTT password reference could not be resolved")
	}
	options.Username, options.Password, options.ServerName, options.PasswordRef = username, password, serverName, ""
	return nil
}
