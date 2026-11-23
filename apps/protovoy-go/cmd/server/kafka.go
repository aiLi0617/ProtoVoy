package main

import (
	"errors"
	"net/http"

	"protovoy.dev/protovoy/internal/execution"
)

type kafkaEndpoint struct {
	target      string
	broker      execution.HostedKafka
	passwordRef string
	tlsRefs     execution.KafkaTLSReferences
}

func configureKafkaEndpoint(target string, options execution.KafkaOptions, caFile, certFile, keyFile string, resolve func(string) (string, error)) (*kafkaEndpoint, error) {
	if target == "" {
		return nil, nil
	}
	if resolve != nil && options.PasswordRef == "" {
		return nil, errors.New("Kafka keychain lookup requires a password reference")
	}
	if options.PasswordRef != "" && (!execution.ValidPasswordReference(options.PasswordRef) || options.SecurityProtocol != "SASL_SSL") {
		return nil, errors.New("a Kafka password reference requires valid SASL_SSL settings")
	}
	refs := options.KafkaTLSReferences
	if !refs.Validate() || refs.CAPEMRef != "" && caFile == "" ||
		refs.CertificatePEMRef != "" && (certFile == "" || keyFile == "") {
		return nil, errors.New("Kafka TLS references require valid names and configured PEM files")
	}
	for _, input := range []struct {
		path, label string
		destination *string
	}{
		{caFile, "CA", &options.CAPEM},
		{certFile, "client certificate", &options.CertificatePEM},
		{keyFile, "client key", &options.KeyPEM},
	} {
		if input.path == "" {
			continue
		}
		data, err := execution.ReadCAFile(input.path)
		if err != nil || len(data) == 0 {
			return nil, errors.New("could not read a nonempty Kafka " + input.label + " file within 1 MiB")
		}
		*input.destination = string(data)
	}
	var passwordLookup func() (string, error)
	if resolve != nil {
		passwordLookup = func() (string, error) { return resolve(options.PasswordRef) }
	}
	broker, err := execution.NewHostedKafka(target, options, passwordLookup)
	if err != nil {
		return nil, err
	}
	return &kafkaEndpoint{target: target, broker: broker, passwordRef: options.PasswordRef, tlsRefs: refs}, nil
}

func kafkaHandler(origin string, endpoint *kafkaEndpoint) http.HandlerFunc {
	if endpoint == nil {
		endpoint = &kafkaEndpoint{}
	}
	return brokerHandler(origin, "Kafka", endpoint.target, func(input execution.KafkaBridgeRequest) string {
		return input.ExpectedTarget
	}, func(input execution.KafkaBridgeRequest) (brokerRunner, error) {
		return endpoint.prepare(input)
	}, func(input execution.KafkaBridgeRequest) int { return input.TimeoutMS })
}

func (endpoint *kafkaEndpoint) prepare(input execution.KafkaBridgeRequest) (brokerRunner, error) {
	if input.ExpectedPasswordRef != "" && input.ExpectedPasswordRef != endpoint.passwordRef {
		return nil, errors.New("Kafka password reference does not match the configured broker")
	}
	refs := input.ExpectedTLSReferences
	if !refs.Validate() || refs.CAPEMRef != "" && refs.CAPEMRef != endpoint.tlsRefs.CAPEMRef ||
		refs.CertificatePEMRef != "" && (refs.CertificatePEMRef != endpoint.tlsRefs.CertificatePEMRef || refs.KeyPEMRef != endpoint.tlsRefs.KeyPEMRef) {
		return nil, errors.New("Kafka TLS references do not match the configured broker")
	}
	request, err := endpoint.broker.Prepare(input.KafkaOperation)
	return request.Execute, err
}
