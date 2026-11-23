package project

import (
	"strings"

	"protovoy.dev/protovoy/internal/execution"
)

type AMQPSettings struct {
	Mode         string `json:"mode"`
	Exchange     string `json:"exchange,omitempty"`
	ExchangeType string `json:"exchangeType"`
	RoutingKey   string `json:"routingKey,omitempty"`
	Queue        string `json:"queue,omitempty"`
	Encoding     string `json:"encoding,omitempty"`
	ContentType  string `json:"contentType,omitempty"`
	Declare      bool   `json:"declare"`
	Durable      bool   `json:"durable,omitempty"`
	AutoAck      bool   `json:"autoAck,omitempty"`
	ReceiveLimit int    `json:"receiveLimit"`
	Username     string `json:"username,omitempty"`
	PasswordRef  string `json:"passwordRef,omitempty"`
	ServerName   string `json:"serverName,omitempty"`
	CAPEM        string `json:"caPem,omitempty"`
}

func (settings AMQPSettings) Valid() bool {
	switch settings.ExchangeType {
	case "direct", "fanout", "topic", "headers":
	default:
		return false
	}
	return execution.ValidPasswordReference(settings.PasswordRef) && (settings.Mode == "publish" || settings.Mode == "consume") &&
		(settings.Encoding == "" || settings.Encoding == "text" || settings.Encoding == "base64") &&
		settings.ReceiveLimit >= 1 && settings.ReceiveLimit <= 10_000
}

func (settings AMQPSettings) HasDestination() bool {
	return settings.Mode == "publish" || strings.TrimSpace(settings.Queue) != ""
}

func (settings AMQPSettings) containsSensitiveData(target string) bool {
	return strings.Contains(target, "@") || hasPrivateKey(settings.CAPEM)
}
