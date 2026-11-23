//go:build !js

package execution

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

type KafkaRequest struct {
	options KafkaOperation
	client  []kgo.Opt
	payload []byte
}

func PrepareKafka(target string, options KafkaOptions) (KafkaRequest, error) {
	return PrepareKafkaWithPasswordResolver(target, options, func(string) (string, error) { return os.Getenv(KafkaPasswordEnv), nil })
}

func PrepareKafkaWithPasswordResolver(target string, options KafkaOptions, resolve func(string) (string, error)) (KafkaRequest, error) {
	if err := configureKafkaPassword(target, &options); err != nil {
		return KafkaRequest{}, err
	}
	if err := resolveKafkaTLSReferences(target, &options); err != nil {
		return KafkaRequest{}, err
	}
	// Named credentials may only be sent to the configured broker addresses.
	restricted := options.PasswordRef != "" || options.KafkaTLSReferences != (KafkaTLSReferences{})
	var passwordLookup func() (string, error)
	if options.PasswordRef != "" {
		if resolve == nil {
			return KafkaRequest{}, errors.New("Kafka password reference could not be resolved")
		}
		passwordLookup = func() (string, error) { return resolve(options.PasswordRef) }
	}
	client, err := kafkaClientOptions(target, options, restricted, passwordLookup)
	if err != nil {
		return KafkaRequest{}, err
	}
	return prepareKafka(options.KafkaOperation, client)
}

type HostedKafka struct {
	client []kgo.Opt
}

func NewHostedKafka(target string, options KafkaOptions, passwordLookup func() (string, error)) (HostedKafka, error) {
	client, err := kafkaClientOptions(target, options, true, passwordLookup)
	return HostedKafka{client}, err
}

func (broker HostedKafka) Prepare(operation KafkaOperation) (KafkaRequest, error) {
	return prepareKafka(operation, broker.client)
}

func prepareKafka(options KafkaOperation, client []kgo.Opt) (KafkaRequest, error) {
	// Hosted requests share the connection settings, then append execution
	// options independently.
	client = client[:len(client):len(client)]
	if options.Mode == "" {
		options.Mode = "produce"
	}
	if options.GroupID == "" {
		options.GroupID = "protovoy-consumer"
	}
	if options.ReceiveLimit == 0 {
		options.ReceiveLimit = 1
	}
	if options.Topic == "" || options.Topic == "." || options.Topic == ".." || len(options.Topic) > 249 ||
		strings.ContainsFunc(options.Topic, func(c rune) bool {
			return !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("._-", c))
		}) {
		return KafkaRequest{}, errors.New("enter a valid Kafka topic")
	}
	if options.Partition != nil && *options.Partition < 0 || options.ReceiveLimit < 1 || options.ReceiveLimit > 10_000 ||
		len(options.GroupID) > 255 || len(options.Key) > 1<<20 || len(options.Payload) > 2<<20 {
		return KafkaRequest{}, errors.New("Kafka request exceeds its partition, group, record or receive limits")
	}
	var payload []byte
	switch options.Mode {
	case "produce":
		switch options.Encoding {
		case "", "text":
			payload = []byte(options.Payload)
		case "base64":
			var err error
			payload, err = base64.StdEncoding.DecodeString(options.Payload)
			if err != nil {
				return KafkaRequest{}, errors.New("enter a valid base64 Kafka record")
			}
		default:
			return KafkaRequest{}, errors.New("Kafka encoding must be text or base64")
		}
		if len(payload)+len(options.Key) > 1<<20 {
			return KafkaRequest{}, errors.New("enter a valid Kafka record within 1 MiB")
		}
		client = append(client, kgo.AllowIdempotentProduceCancellation())
		if options.Partition != nil {
			client = append(client, kgo.RecordPartitioner(kgo.ManualPartitioner()))
		}
	case "consume":
		offset := kgo.NewOffset()
		switch options.OffsetReset {
		case "", "latest":
			offset = offset.AtEnd()
		case "earliest":
			offset = offset.AtStart()
		case "error":
			offset = offset.AtCommitted()
		default:
			return KafkaRequest{}, errors.New("Kafka offset reset must be earliest, latest, or error")
		}
		client = append(client, kgo.ConsumeTopics(options.Topic), kgo.ConsumerGroup(options.GroupID),
			kgo.ConsumeStartOffset(offset), kgo.ConsumeResetOffset(offset), kgo.BlockRebalanceOnPoll(),
			kgo.FetchMaxBytes(4<<20), kgo.FetchMaxPartitionBytes(2<<20), kgo.MaxDecompressBatchBytes(8<<20))
		if options.AutoCommit {
			client = append(client, kgo.AutoCommitMarks())
		} else {
			client = append(client, kgo.DisableAutoCommit())
		}
	default:
		return KafkaRequest{}, errors.New("Kafka mode must be produce or consume")
	}
	return KafkaRequest{options, client, payload}, nil
}

func kafkaClientOptions(target string, options KafkaOptions, hosted bool, passwordLookup func() (string, error)) ([]kgo.Opt, error) {
	brokers, err := kafkaBrokers(target)
	if err != nil {
		return nil, err
	}
	config, sasl, err := kafkaSecurity(options, passwordLookup)
	if err != nil {
		return nil, err
	}
	client := []kgo.Opt{kgo.SeedBrokers(brokers...), kgo.ClientID("protovoy"), kgo.RequestTimeoutOverhead(5 * time.Second)}
	switch {
	case hosted:
		client = append(client, kgo.Dialer(hostedKafkaDialer(brokers, config)))
	case config != nil:
		client = append(client, kgo.DialTLSConfig(config))
	}
	return append(client, sasl...), nil
}

func hostedKafkaDialer(brokers []string, config *tls.Config) func(context.Context, string, string) (net.Conn, error) {
	allowed := make(map[string]bool, len(brokers))
	for _, broker := range brokers {
		allowed[broker] = true
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	dial := dialer.DialContext
	if config != nil {
		dial = (&tls.Dialer{NetDialer: dialer, Config: config}).DialContext
	}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		number, portErr := strconv.ParseUint(port, 10, 16)
		if err != nil || portErr != nil || !allowed[kafkaAddress(host, number)] {
			return nil, errors.New("Kafka broker is outside the configured address list")
		}
		return dial(ctx, network, kafkaAddress(host, number))
	}
}

func kafkaAddress(host string, port uint64) string {
	if ip, err := netip.ParseAddr(host); err == nil {
		host = ip.String()
	} else {
		host = strings.ToLower(host)
	}
	return net.JoinHostPort(host, strconv.FormatUint(port, 10))
}

func kafkaBrokers(target string) ([]string, error) {
	invalid := errors.New("enter Kafka brokers as a comma-separated list of host:port addresses")
	brokers := strings.Split(strings.TrimPrefix(strings.TrimSpace(target), "kafka://"), ",")
	if len(brokers) > 16 {
		return nil, invalid
	}
	for i, broker := range brokers {
		broker = strings.TrimSpace(broker)
		host, port, err := net.SplitHostPort(broker)
		number, portErr := strconv.ParseUint(port, 10, 16)
		if err != nil || host == "" || portErr != nil || number == 0 || strings.ContainsAny(host, "@/\\?#\r\n\x00 \t") {
			return nil, invalid
		}
		brokers[i] = kafkaAddress(host, number)
	}
	return brokers, nil
}

func kafkaSecurity(options KafkaOptions, passwordLookup func() (string, error)) (*tls.Config, []kgo.Opt, error) {
	protocol := options.SecurityProtocol
	credentials := options.Username != "" || options.Password != ""
	switch protocol {
	case "", "PLAINTEXT":
		if credentials || options.CAPEM != "" || options.CertificatePEM != "" || options.KeyPEM != "" {
			return nil, nil, errors.New("Kafka credentials and certificates require TLS")
		}
		return nil, nil, nil
	case "SSL", "SASL_SSL":
	default:
		return nil, nil, errors.New("Kafka security protocol must be PLAINTEXT, SSL, or SASL_SSL")
	}
	config := &tls.Config{MinVersion: tls.VersionTLS12}
	if options.CAPEM != "" {
		roots, err := socketRoots(options.CAPEM, nil)
		if err != nil {
			return nil, nil, err
		}
		config.RootCAs = roots
	}
	if options.CertificatePEM != "" || options.KeyPEM != "" {
		if len(options.CertificatePEM)+len(options.KeyPEM) > 1<<20 {
			return nil, nil, errors.New("Kafka client certificate and key exceed 1 MiB")
		}
		pair, err := tls.X509KeyPair([]byte(options.CertificatePEM), []byte(options.KeyPEM))
		if err != nil {
			return nil, nil, errors.New("invalid Kafka client certificate or key")
		}
		config.Certificates = []tls.Certificate{pair}
	}
	if protocol == "SSL" {
		if credentials {
			return nil, nil, errors.New("Kafka username and password require SASL_SSL")
		}
		return config, nil, nil
	}
	if options.Username == "" {
		return nil, nil, errors.New("Kafka SASL requires a username and password")
	}
	switch options.SASLMechanism {
	case "", "PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512":
	default:
		return nil, nil, errors.New("Kafka SASL mechanism must be PLAIN, SCRAM-SHA-256, or SCRAM-SHA-512")
	}
	if passwordLookup != nil {
		password, err := passwordLookup()
		if err != nil || password == "" {
			return nil, nil, errors.New("Kafka password reference could not be resolved")
		}
		options.Password = password
	}
	if options.Password == "" {
		return nil, nil, errors.New("Kafka SASL requires a username and password")
	}
	auth := scram.Auth{User: options.Username, Pass: options.Password}
	var sasl kgo.Opt
	switch options.SASLMechanism {
	case "", "PLAIN":
		sasl = kgo.SASL((plain.Auth{User: options.Username, Pass: options.Password}).AsMechanism())
	case "SCRAM-SHA-256":
		sasl = kgo.SASL(auth.AsSha256Mechanism())
	case "SCRAM-SHA-512":
		sasl = kgo.SASL(auth.AsSha512Mechanism())
	}
	return config, []kgo.Opt{sasl}, nil
}

func (request KafkaRequest) Execute(ctx context.Context, connected func() error, emit func(any) error) error {
	client, err := kgo.NewClient(request.client...)
	if err != nil {
		return errors.New("could not configure Kafka client")
	}
	defer client.CloseAllowingRebalance()
	if err := client.Ping(ctx); err != nil {
		return kafkaFailure(ctx, "could not connect to Kafka broker")
	}
	if err := connected(); err != nil {
		return err
	}
	options := request.options
	if options.Mode == "produce" {
		record := &kgo.Record{Topic: options.Topic, Value: request.payload}
		if options.Key != "" {
			record.Key = []byte(options.Key)
		}
		if options.Partition != nil {
			record.Partition = *options.Partition
		}
		if client.ProduceSync(ctx, record).FirstErr() != nil {
			return kafkaFailure(ctx, "Kafka produce failed")
		}
		return emit(map[string]any{
			"produced": true, "topic": record.Topic, "partition": record.Partition, "offset": record.Offset,
			"timestamp": record.Timestamp.UTC(), "bytes": len(record.Value),
		})
	}
	for range options.ReceiveLimit {
		fetches := client.PollRecords(ctx, 1)
		if fetches.Err() != nil || fetches.NumRecords() != 1 {
			return kafkaFailure(ctx, "Kafka consume failed")
		}
		record := fetches.RecordIter().Next()
		if len(record.Key)+len(record.Value) > 1<<20 {
			return errors.New("Kafka record exceeds 1 MiB")
		}
		if err := emit(kafkaRecord(record)); err != nil {
			return err
		}
		if options.AutoCommit {
			client.MarkCommitRecords(record)
		} else if client.CommitRecords(ctx, record) != nil {
			return kafkaFailure(ctx, "Kafka offset commit failed")
		}
		client.AllowRebalance()
	}
	if options.AutoCommit && client.CommitMarkedOffsets(ctx) != nil {
		return kafkaFailure(ctx, "Kafka offset commit failed")
	}
	return nil
}

func kafkaRecord(record *kgo.Record) map[string]any {
	result := map[string]any{
		"topic": record.Topic, "partition": record.Partition, "offset": record.Offset, "timestamp": record.Timestamp.UTC(),
		"key": nil, "keyBase64": nil, "payload": nil, "payloadBase64": base64.StdEncoding.EncodeToString(record.Value),
	}
	if record.Key != nil {
		result["keyBase64"] = base64.StdEncoding.EncodeToString(record.Key)
		if utf8.Valid(record.Key) {
			result["key"] = string(record.Key)
		}
	}
	if utf8.Valid(record.Value) {
		result["payload"] = string(record.Value)
	}
	return result
}

func kafkaFailure(ctx context.Context, message string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return errors.New(message)
}
