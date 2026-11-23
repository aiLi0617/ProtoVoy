package main

import (
	"errors"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/secrets"
)

func (envelope requestEnvelope) prepareKafka(useKeychain bool) (execution.KafkaRequest, error) {
	var input struct {
		Type  string `json:"type"`
		Value *struct {
			execution.KafkaOptions
			KeyPasswordRef string
			Literal        bool
		} `json:"value"`
	}
	if !decodeExecutionPayload(envelope.Payload, &input) || input.Type != "raw" || input.Value == nil {
		return execution.KafkaRequest{}, errors.New("invalid Kafka execution payload")
	}
	value := input.Value
	if value.KeyPasswordRef != "" {
		return execution.KafkaRequest{}, errors.New("Kafka private key password references are unavailable")
	}
	target := envelope.Target
	options := value.KafkaOptions
	if !value.Literal {
		var err error
		target, err = options.ResolveVariables(target, envelope.Variables)
		if err != nil {
			return execution.KafkaRequest{}, err
		}
	}
	if useKeychain {
		return execution.PrepareKafkaWithPasswordResolver(target, options, secrets.Resolve)
	}
	return execution.PrepareKafka(target, options)
}
