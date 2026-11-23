package main

import (
	"context"
	"errors"
	"io"
	"math"
	"os"
	"time"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/secrets"
)

func prepareCLIKafka(target string, options execution.KafkaOptions) (execution.KafkaRequest, error) {
	if os.Getenv("PROTOVOY_KAFKA_PASSWORD_KEYCHAIN") == "1" {
		return execution.PrepareKafkaWithPasswordResolver(target, options, secrets.Resolve)
	}
	return execution.PrepareKafka(target, options)
}

func runKafka(ctx context.Context, target string, options execution.KafkaOptions, partition int64, caFile string, timeout time.Duration, output io.Writer) error {
	if partition < -1 || partition > math.MaxInt32 {
		return errors.New("Kafka partition must be -1 or a nonnegative 32-bit integer")
	}
	if options.Mode == "consume" && (options.Payload != "" || options.Key != "" || partition != -1) {
		return errors.New("-data, -kafka-key and -kafka-partition apply only to Kafka produce")
	}
	if options.Mode == "produce" && (options.GroupID != "" || options.OffsetReset != "" || options.ReceiveLimit != 1) {
		return errors.New("Kafka consumer options require -kafka-consume")
	}
	if partition >= 0 {
		selected := int32(partition)
		options.Partition = &selected
	}
	if caFile != "" && (options.PasswordRef != "" || options.KafkaTLSReferences != (execution.KafkaTLSReferences{})) {
		return errors.New("-ca cannot be combined with Kafka credential references")
	}
	var err error
	options.CAPEM, err = readBrokerCA(caFile)
	if err != nil {
		return err
	}
	request, err := prepareCLIKafka(target, options)
	if err != nil {
		return err
	}
	return runBroker(ctx, timeout, output, request.Execute)
}
