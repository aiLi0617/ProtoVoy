package project

import (
	"strings"

	"protovoy.dev/protovoy/internal/execution"
)

type MQTTSettings struct {
	Mode              string `json:"mode"`
	Topic             string `json:"topic"`
	Encoding          string `json:"encoding,omitempty"`
	QoS               int    `json:"qos"`
	Retain            bool   `json:"retain,omitempty"`
	ReceiveLimit      int    `json:"receiveLimit"`
	ClientID          string `json:"clientId,omitempty"`
	Username          string `json:"username,omitempty"`
	PasswordRef       string `json:"passwordRef,omitempty"`
	PersistentSession bool   `json:"persistentSession,omitempty"`
	KeepAlive         int    `json:"keepAliveSeconds"`
	ServerName        string `json:"serverName,omitempty"`
	CAPEM             string `json:"caPem,omitempty"`
}

func (settings MQTTSettings) Valid() bool {
	return execution.ValidPasswordReference(settings.PasswordRef) && (settings.Mode == "publish" || settings.Mode == "subscribe") &&
		(settings.Encoding == "" || settings.Encoding == "text" || settings.Encoding == "base64") &&
		settings.QoS >= 0 && settings.QoS <= 2 && settings.ReceiveLimit >= 1 && settings.ReceiveLimit <= 10_000 &&
		settings.KeepAlive >= 0 && settings.KeepAlive <= 65535
}

func (settings MQTTSettings) containsSensitiveData(target string) bool {
	return strings.Contains(target, "@") || hasPrivateKey(settings.CAPEM)
}
