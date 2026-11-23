package project

import (
	"errors"
	"strconv"
	"strings"

	"protovoy.dev/protovoy/internal/execution"
)

type KafkaSettings struct {
	execution.KafkaTLSReferences
	Mode             string `json:"mode"`
	Topic            string `json:"topic"`
	Key              string `json:"key,omitempty"`
	Encoding         string `json:"encoding,omitempty"`
	Partition        string `json:"partition,omitempty"`
	GroupID          string `json:"groupId,omitempty"`
	OffsetReset      string `json:"offsetReset,omitempty"`
	ReceiveLimit     string `json:"receiveLimit,omitempty"`
	AutoCommit       bool   `json:"autoCommit,omitempty"`
	SecurityProtocol string `json:"securityProtocol,omitempty"`
	SASLMechanism    string `json:"saslMechanism,omitempty"`
	Username         string `json:"username,omitempty"`
	PasswordRef      string `json:"passwordRef,omitempty"`
}

func (settings KafkaSettings) Validate(target, payload string) error {
	if settings.containsSensitiveData(target) {
		return errors.New("remove possible credentials from the Kafka request")
	}
	_, err := settings.Operation(payload)
	return err
}

func (settings KafkaSettings) containsSensitiveData(target string) bool {
	return strings.Contains(target, "@") || sensitiveBody.MatchString(settings.Key) || hasPrivateKey(settings.Key)
}

func (settings KafkaSettings) Operation(payload string) (execution.KafkaOperation, error) {
	if !settings.KafkaTLSReferences.Validate() {
		return execution.KafkaOperation{}, errors.New("enter valid Kafka TLS references and a certificate/key pair")
	}
	if !execution.ValidPasswordReference(settings.PasswordRef) {
		return execution.KafkaOperation{}, errors.New("enter a valid Kafka password reference")
	}
	if (settings.Mode != "produce" && settings.Mode != "consume") || strings.TrimSpace(settings.Topic) == "" {
		return execution.KafkaOperation{}, errors.New("select a Kafka operation and enter a topic")
	}
	input := execution.KafkaOperation{Mode: settings.Mode, Topic: settings.Topic}
	if settings.Mode == "produce" {
		input.Key, input.Encoding, input.Payload = settings.Key, settings.Encoding, payload
		if settings.Partition != "" {
			partition, err := strconv.ParseInt(settings.Partition, 10, 32)
			if err != nil || partition < 0 {
				return input, errors.New("enter a nonnegative Kafka partition")
			}
			value := int32(partition)
			input.Partition = &value
		}
	} else {
		input.GroupID, input.OffsetReset, input.AutoCommit = settings.GroupID, settings.OffsetReset, settings.AutoCommit
		if settings.ReceiveLimit != "" {
			limit, err := strconv.Atoi(settings.ReceiveLimit)
			if err != nil || limit < 1 || limit > 10_000 {
				return input, errors.New("enter a Kafka receive limit from 1 to 10,000")
			}
			input.ReceiveLimit = limit
		}
	}
	return input, nil
}
