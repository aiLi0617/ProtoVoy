//go:build !js

package execution

import (
	"errors"
	"os"
)

func configureKafkaPassword(target string, options *KafkaOptions) error {
	if options.PasswordRef == "" {
		return nil
	}
	if options.Password != "" || options.CAPEM != "" {
		return errors.New("Kafka password references use the configured password and CA")
	}
	username := os.Getenv("PROTOVOY_KAFKA_USERNAME")
	mechanism := os.Getenv("PROTOVOY_KAFKA_SASL")
	if mechanism == "" {
		mechanism = "PLAIN"
	}
	if !ValidPasswordReference(options.PasswordRef) || options.PasswordRef != os.Getenv(KafkaPasswordRefEnv) ||
		target != os.Getenv("PROTOVOY_KAFKA_TARGET") || username == "" ||
		(options.Username != "" && options.Username != username) ||
		(options.SASLMechanism != "" && options.SASLMechanism != mechanism) ||
		options.SecurityProtocol != "SASL_SSL" {
		return errors.New("Kafka password reference is unavailable for these connection settings")
	}
	options.Username, options.SASLMechanism = username, mechanism
	return nil
}

func resolveKafkaTLSReferences(target string, options *KafkaOptions) error {
	refs := options.KafkaTLSReferences
	if !refs.Validate() {
		return errors.New("Kafka TLS references require valid names and a certificate/key pair")
	}
	if refs == (KafkaTLSReferences{}) && options.PasswordRef == "" {
		return nil
	}
	if target != os.Getenv("PROTOVOY_KAFKA_TARGET") ||
		(options.SecurityProtocol != "SSL" && options.SecurityProtocol != "SASL_SSL") || options.CAPEM != "" {
		return errors.New("Kafka credential references require the configured broker and TLS trust")
	}
	for _, material := range []struct {
		label, reference, refEnv, fileEnv string
		destination                       *string
		useConfiguredDefault              bool
	}{
		{"CA", refs.CAPEMRef, "PROTOVOY_KAFKA_CA_REF", "PROTOVOY_KAFKA_CA_FILE", &options.CAPEM, true},
		{"client certificate", refs.CertificatePEMRef, "PROTOVOY_KAFKA_CERT_REF", "PROTOVOY_KAFKA_CERT_FILE", &options.CertificatePEM, false},
		{"private key", refs.KeyPEMRef, "PROTOVOY_KAFKA_KEY_REF", "PROTOVOY_KAFKA_KEY_FILE", &options.KeyPEM, false},
	} {
		path := os.Getenv(material.fileEnv)
		if material.reference != "" {
			if material.reference != os.Getenv(material.refEnv) || path == "" || *material.destination != "" {
				return errors.New("Kafka " + material.label + " reference is unavailable or conflicts with session credentials")
			}
		} else if !material.useConfiguredDefault || path == "" {
			continue
		}
		data, err := ReadCAFile(path)
		if err != nil || len(data) == 0 {
			return errors.New("could not load the configured Kafka " + material.label)
		}
		*material.destination = string(data)
	}
	return nil
}
