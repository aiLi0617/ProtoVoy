package main

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"protovoy.dev/protovoy/internal/buildinfo"
	"protovoy.dev/protovoy/internal/execution"
)

func main() {
	if len(os.Args) > 2 && os.Args[1] == "plugin" && os.Args[2] == "remove" {
		if err := removePlugin(os.Args[3:], os.Getenv("PROTOVOY_PLUGIN_DIR"), os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 2 && os.Args[1] == "plugin" && (os.Args[2] == "enable" || os.Args[2] == "disable") {
		if err := setPluginEnabled(os.Args[3:], os.Args[2] == "enable", os.Getenv("PROTOVOY_PLUGIN_DIR"), os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "plugin" && os.Args[2] == "list" {
		if err := listPlugins(os.Getenv("PROTOVOY_PLUGIN_DIR"), os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 2 && os.Args[1] == "plugin" && (os.Args[2] == "install" || os.Args[2] == "update") {
		if err := installPlugin(os.Args[3:], os.Getenv("PROTOVOY_PLUGIN_TRUSTED_KEYS"), os.Getenv("PROTOVOY_PLUGIN_DIR"), os.Args[2] == "update", os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 2 && os.Args[1] == "plugin" && (os.Args[2] == "run" || os.Args[2] == "import") {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		run := executePlugin
		if os.Args[2] == "import" {
			run = importPluginProject
		}
		err := run(ctx, os.Args[3:], os.Getenv("PROTOVOY_PLUGIN_TRUSTED_KEYS"), os.Stdin, os.Stdout)
		stop()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 2 && os.Args[1] == "plugin" && os.Args[2] == "inspect" {
		if err := inspectPlugin(os.Args[3:], os.Getenv("PROTOVOY_PLUGIN_TRUSTED_KEYS"), os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "secret" {
		secretCommand(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "import-curl" {
		if len(os.Args) != 2 {
			fmt.Fprintln(os.Stderr, "Usage: protovoy import-curl < request.txt")
			os.Exit(2)
		}
		if err := importCurl(os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "curl" {
		runCurlCommand(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "run" {
		runCollectionCommand(os.Args[2:])
		return
	}
	version := flag.Bool("version", false, "print the application version")
	method := flag.String("method", "GET", "HTTP method")
	network := flag.String("protocol", "http", "http, sse, websocket, tcp, udp, redis, grpc, graphql, jsonrpc, soap, sql, mqtt, amqp, or kafka")
	passwordSecret := flag.String("password-secret", "", "system credential reference for the protocol password")
	data := flag.String("data", "", "request body, protocol document, or command data")
	dataFile := flag.String("data-file", "", "read request data from a file or - for stdin (up to 8 MiB)")
	var headerValues []string
	var headerSecrets []string
	flag.Func("header-secret", "request header in 'Name: secret-reference' form (repeatable)", func(value string) error {
		headerSecrets = append(headerSecrets, value)
		return nil
	})
	flag.Func("header", "HTTP request or WebSocket handshake header in 'Name: value' form (repeatable)", func(value string) error {
		headerValues = append(headerValues, value)
		return nil
	})
	sseReconnects := flag.String("sse-reconnects", "", "SSE reconnect attempts after a clean disconnect (0 to 10)")
	sseDelay := flag.String("sse-reconnect-delay-ms", "", "initial SSE reconnect delay in milliseconds")
	wsProtocols := flag.String("ws-subprotocols", "", "comma-separated WebSocket subprotocols")
	wsReceiveLimit := flag.Int("ws-receive-limit", 0, "stop after this many WebSocket messages (0 waits for closure)")
	graphqlVariables := flag.String("graphql-variables", "", "GraphQL variables as a JSON object")
	graphqlOperation := flag.String("graphql-operation", "", "GraphQL operation name")
	graphqlSubscribe := flag.Bool("graphql-subscribe", false, "use graphql-transport-ws and stream JSON results")
	rpcMethod := flag.String("rpc-method", "", "JSON-RPC method name")
	rpcID := flag.String("rpc-id", "", "JSON-RPC ID as JSON (default 1)")
	soapVersion := flag.String("soap-version", "", "SOAP version, 1.1 or 1.2 (default 1.2)")
	soapAction := flag.String("soap-action", "", "SOAPAction header value")
	sqlUsername := flag.String("sql-username", "", "SQL username (password from "+execution.SQLPasswordEnv+")")
	sqlPasswordRef := flag.String("sql-password-ref", "", "configured SQL password reference")
	sqlParameters := flag.String("sql-parameters", "", "SQL parameters as a JSON array")
	sqlRowLimit := flag.Int("sql-row-limit", 500, "maximum SQL rows to return (1 to 10000)")
	sqlTransaction := flag.Bool("sql-transaction", false, "execute the SQL statement in a transaction")
	mqttSubscribe := flag.Bool("mqtt-subscribe", false, "subscribe to an MQTT topic filter")
	mqttTopic := flag.String("mqtt-topic", "", "MQTT topic or subscription filter")
	mqttQoS := flag.Int("mqtt-qos", 0, "MQTT QoS (0, 1, or 2)")
	mqttRetain := flag.Bool("mqtt-retain", false, "retain the MQTT publication")
	mqttReceiveLimit := flag.Int("mqtt-receive-limit", 1, "number of MQTT messages to receive (1 to 10000)")
	mqttClientID := flag.String("mqtt-client-id", "", "MQTT client ID")
	mqttUsername := flag.String("mqtt-username", "", "MQTT username (password from "+execution.MQTTPasswordEnv+")")
	mqttPasswordRef := flag.String("mqtt-password-ref", "", "configured MQTT password reference")
	amqpConsume := flag.Bool("amqp-consume", false, "consume messages from an AMQP queue")
	amqpExchange := flag.String("amqp-exchange", "", "AMQP exchange (empty uses the default exchange)")
	amqpExchangeType := flag.String("amqp-exchange-type", "direct", "AMQP exchange type for declaration")
	amqpRoutingKey := flag.String("amqp-routing-key", "", "AMQP routing key")
	amqpQueue := flag.String("amqp-queue", "", "AMQP queue to consume")
	amqpDeclare := flag.Bool("amqp-declare", false, "declare the named exchange and, for consume, the queue and binding")
	amqpDurable := flag.Bool("amqp-durable", false, "use durable declarations and persistent publications")
	amqpReceiveLimit := flag.Int("amqp-receive-limit", 1, "number of AMQP messages to receive (1 to 10000)")
	amqpUsername := flag.String("amqp-username", "", "AMQP username (password from "+execution.AMQPPasswordEnv+")")
	amqpPasswordRef := flag.String("amqp-password-ref", "", "configured AMQP password reference")
	kafkaConsume := flag.Bool("kafka-consume", false, "consume records from a Kafka topic")
	kafkaTopic := flag.String("kafka-topic", "", "Kafka topic")
	kafkaKey := flag.String("kafka-key", "", "Kafka record key")
	kafkaPartition := flag.Int64("kafka-partition", -1, "Kafka partition for produce (-1 selects automatically)")
	kafkaGroup := flag.String("kafka-group", "", "Kafka consumer group")
	kafkaOffset := flag.String("kafka-offset-reset", "", "Kafka offset reset: earliest, latest (default), or error")
	kafkaReceiveLimit := flag.Int("kafka-receive-limit", 1, "number of Kafka records to consume (1 to 10000)")
	kafkaSecurity := flag.String("kafka-security", "", "Kafka security protocol: PLAINTEXT (default), SSL, or SASL_SSL")
	kafkaSASL := flag.String("kafka-sasl", "", "Kafka SASL mechanism: PLAIN, SCRAM-SHA-256, or SCRAM-SHA-512")
	kafkaUsername := flag.String("kafka-username", "", "Kafka SASL username (password from "+execution.KafkaPasswordEnv+")")
	kafkaPasswordRef := flag.String("kafka-password-ref", "", "configured Kafka SASL password reference")
	kafkaCARef := flag.String("kafka-ca-ref", "", "configured Kafka CA reference")
	kafkaCertRef := flag.String("kafka-cert-ref", "", "configured Kafka client certificate reference")
	kafkaKeyRef := flag.String("kafka-key-ref", "", "configured Kafka client private key reference")
	grpcService := flag.String("grpc-service", "", "fully qualified gRPC service name")
	grpcMethod := flag.String("grpc-method", "", "gRPC method name")
	grpcMode := flag.String("grpc-mode", "unary", "unary, server_streaming, client_streaming, or bidi_streaming")
	grpcDescriptor := flag.String("grpc-descriptor", "", "binary gRPC descriptor set file (protoc --include_imports)")
	grpcJSON := flag.Bool("grpc-json", false, "interpret -data as ProtoJSON")
	redisPipeline := flag.Bool("redis-pipeline", false, "interpret -data as a JSON array of Redis commands")
	redisProtocol := flag.Int("redis-resp", 2, "Redis protocol version (2 or 3)")
	redisUsername := flag.String("redis-username", "", "Redis ACL username (password from "+execution.RedisPasswordEnv+")")
	redisPasswordRef := flag.String("redis-password-ref", "", "configured Redis password reference")
	redisDatabase := flag.String("redis-db", "", "Redis database number")
	encoding := flag.String("encoding", "text", "socket payload encoding: text, base64, or hex (TCP/UDP only for hex)")
	framing := flag.String("framing", "raw", "TCP response framing: raw, fixed, delimiter")
	delimiter := flag.String("delimiter", "", "TCP response delimiter")
	length := flag.Int("length", 0, "fixed TCP response length")
	count := flag.Int("count", 1, "socket sends on the same connection (1 to 10000)")
	interval := flag.Duration("interval", 0, "interval between socket sends")
	useTLS := flag.Bool("tls", false, "enable verified TCP TLS")
	serverName := flag.String("server-name", "", "TLS certificate name and SNI")
	caFile := flag.String("ca", "", "PEM CA file for TLS")
	timeout := flag.Duration("timeout", 30*time.Second, "request timeout")
	follow := flag.Bool("follow", false, "follow redirects")
	preScript := flag.String("pre-script", "", "JavaScript file to run before an HTTP request")
	postScript := flag.String("post-script", "", "JavaScript file to run after an HTTP response")
	responsePlugin := flag.String("response-plugin", "", "trusted transformer package directory for the HTTP response body")
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "Usage: protovoy [options] URL")
		fmt.Fprintln(flag.CommandLine.Output(), "       protovoy run [options] PROJECT.json")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *version {
		fmt.Println("ProtoVoy CLI " + buildinfo.Version)
		return
	}
	if flag.NArg() != 1 || *timeout <= 0 {
		flag.Usage()
		os.Exit(2)
	}
	var hasData, hasDataFile bool
	flag.Visit(func(option *flag.Flag) {
		hasData = hasData || option.Name == "data"
		hasDataFile = hasDataFile || option.Name == "data-file"
	})
	if hasDataFile {
		if hasData || *dataFile == "" || *network == "sse" {
			fmt.Fprintln(os.Stderr, "-data-file requires a path or - and cannot be used with -data or SSE")
			os.Exit(2)
		}
		body, err := readCLIData(*dataFile, os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		*data = body
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	var password string
	var err error
	switch {
	case *sqlPasswordRef != "" && *network != "sql":
		err = fmt.Errorf("-sql-password-ref requires -protocol sql")
	case *kafkaPasswordRef != "" && *network != "kafka":
		err = fmt.Errorf("-kafka-password-ref requires -protocol kafka")
	case *sqlPasswordRef != "" && *passwordSecret != "":
		err = fmt.Errorf("SQL password references cannot be combined with -password-secret")
	case *kafkaPasswordRef != "" && *passwordSecret != "":
		err = fmt.Errorf("Kafka password references cannot be combined with -password-secret")
	case *sqlPasswordRef == "" && *kafkaPasswordRef == "":
		password, err = protocolPassword(*network, *passwordSecret)
	}
	if *network == "redis" && *redisPasswordRef != "" {
		password = ""
		if *passwordSecret != "" {
			err = fmt.Errorf("Redis password references cannot be combined with -password-secret")
		}
	}
	if *network == "amqp" && *amqpPasswordRef != "" {
		password = ""
		if *passwordSecret != "" {
			err = fmt.Errorf("AMQP password references cannot be combined with -password-secret")
		}
	}
	if *network == "mqtt" && *mqttPasswordRef != "" {
		password = ""
		if *passwordSecret != "" {
			err = fmt.Errorf("MQTT password references cannot be combined with -password-secret")
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		stop()
		os.Exit(2)
	}
	headerValues, err = addSecretHeaders(headerValues, headerSecrets)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		stop()
		os.Exit(2)
	}
	headers, err := parseCLIHeaders(headerValues)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		stop()
		os.Exit(2)
	}
	if *responsePlugin != "" && (*network != "http" || len(headerSecrets) != 0 || *preScript != "" || *postScript != "") {
		err = fmt.Errorf("-response-plugin requires HTTP without request scripts or -header-secret")
	} else if (*preScript != "" || *postScript != "") && (*network != "http" || len(headerSecrets) != 0) {
		err = fmt.Errorf("HTTP scripts require -protocol http and cannot be combined with -header-secret")
	} else if len(headers) != 0 && *network != "http" && *network != "sse" && *network != "websocket" && *network != "graphql" && *network != "jsonrpc" && *network != "soap" {
		err = fmt.Errorf("-header requires -protocol http, sse, websocket, graphql, jsonrpc, or soap")
	} else if (*sseReconnects != "" || *sseDelay != "") && *network != "sse" {
		err = fmt.Errorf("SSE options require -protocol sse")
	} else if (*wsProtocols != "" || *wsReceiveLimit != 0) && *network != "websocket" {
		err = fmt.Errorf("WebSocket options require -protocol websocket")
	} else if (*redisPipeline || *redisPasswordRef != "") && *network != "redis" {
		err = fmt.Errorf("-redis-pipeline and -redis-password-ref require -protocol redis")
	} else if (*grpcDescriptor != "" || *grpcJSON) && *network != "grpc" {
		err = fmt.Errorf("-grpc-descriptor and -grpc-json require -protocol grpc")
	} else if (*graphqlVariables != "" || *graphqlOperation != "" || *graphqlSubscribe) && *network != "graphql" {
		err = fmt.Errorf("GraphQL options require -protocol graphql")
	} else if (*rpcMethod != "" || *rpcID != "") && *network != "jsonrpc" {
		err = fmt.Errorf("JSON-RPC options require -protocol jsonrpc")
	} else if (*soapVersion != "" || *soapAction != "") && *network != "soap" {
		err = fmt.Errorf("SOAP options require -protocol soap")
	} else if (*sqlUsername != "" || *sqlParameters != "" || *sqlRowLimit != 500 || *sqlTransaction) && *network != "sql" {
		err = fmt.Errorf("SQL options require -protocol sql")
	} else if (*mqttSubscribe || *mqttTopic != "" || *mqttQoS != 0 || *mqttRetain || *mqttReceiveLimit != 1 ||
		*mqttClientID != "" || *mqttUsername != "" || *mqttPasswordRef != "") && *network != "mqtt" {
		err = fmt.Errorf("MQTT options require -protocol mqtt")
	} else if (*amqpConsume || *amqpExchange != "" || *amqpExchangeType != "direct" || *amqpRoutingKey != "" ||
		*amqpQueue != "" || *amqpDeclare || *amqpDurable || *amqpReceiveLimit != 1 || *amqpUsername != "" || *amqpPasswordRef != "") && *network != "amqp" {
		err = fmt.Errorf("AMQP options require -protocol amqp")
	} else if (*kafkaConsume || *kafkaTopic != "" || *kafkaKey != "" || *kafkaPartition != -1 || *kafkaGroup != "" ||
		*kafkaOffset != "" || *kafkaReceiveLimit != 1 || *kafkaSecurity != "" || *kafkaSASL != "" || *kafkaUsername != "" ||
		*kafkaCARef != "" || *kafkaCertRef != "" || *kafkaKeyRef != "") && *network != "kafka" {
		err = fmt.Errorf("Kafka options require -protocol kafka")
	} else if *network == "kafka" {
		options := execution.KafkaOptions{
			KafkaOperation: execution.KafkaOperation{
				Mode: "produce", Topic: *kafkaTopic, Key: *kafkaKey, Payload: *data, Encoding: *encoding,
				GroupID: *kafkaGroup, OffsetReset: *kafkaOffset, ReceiveLimit: *kafkaReceiveLimit,
			},
			SecurityProtocol: *kafkaSecurity, SASLMechanism: *kafkaSASL,
			Username: *kafkaUsername, Password: password, PasswordRef: *kafkaPasswordRef,
			KafkaTLSReferences: execution.KafkaTLSReferences{
				CAPEMRef: *kafkaCARef, CertificatePEMRef: *kafkaCertRef, KeyPEMRef: *kafkaKeyRef,
			},
		}
		if *kafkaConsume {
			options.Mode = "consume"
		}
		err = runKafka(ctx, flag.Arg(0), options, *kafkaPartition, *caFile, *timeout, os.Stdout)
	} else if *network == "amqp" {
		options := execution.AMQPOptions{
			Mode: "publish", Exchange: *amqpExchange, ExchangeType: *amqpExchangeType,
			RoutingKey: *amqpRoutingKey, Queue: *amqpQueue, Declare: *amqpDeclare, Durable: *amqpDurable,
			Payload: *data, Encoding: *encoding, ReceiveLimit: *amqpReceiveLimit,
			Username: *amqpUsername, Password: password, ServerName: *serverName,
			PasswordRef: *amqpPasswordRef,
		}
		if *amqpConsume {
			options.Mode = "consume"
		}
		err = runAMQP(ctx, flag.Arg(0), options, *caFile, *timeout, os.Stdout)
	} else if *network == "mqtt" {
		options := execution.MQTTOptions{
			Mode: "publish", Topic: *mqttTopic, Payload: *data, Encoding: *encoding,
			QoS: *mqttQoS, Retain: *mqttRetain, ReceiveLimit: *mqttReceiveLimit,
			ClientID: *mqttClientID, Username: *mqttUsername, Password: password,
			PasswordRef: *mqttPasswordRef,
			ServerName:  *serverName, CleanSession: true, KeepAlive: 30,
		}
		if *mqttSubscribe {
			options.Mode = "subscribe"
		}
		err = runMQTT(ctx, flag.Arg(0), options, *caFile, *timeout, os.Stdout)
	} else if *network == "sql" {
		err = runSQL(ctx, flag.Arg(0), *data, execution.SQLCredentials{Username: *sqlUsername, Password: password, PasswordRef: *sqlPasswordRef},
			*sqlParameters, *sqlRowLimit, *sqlTransaction, *timeout, os.Stdout)
	} else if *network == "sse" {
		err = runSSE(ctx, flag.Arg(0), headers, *sseReconnects, *sseDelay, *timeout, os.Stdout, os.Stderr)
	} else if *network == "websocket" {
		options := execution.WebSocketAutomation{}
		if *wsReceiveLimit != 0 {
			options.ReceiveLimit = wsReceiveLimit
		}
		if hasData || hasDataFile {
			messageEncoding := *encoding
			if messageEncoding == "base64" {
				messageEncoding = "binary"
			}
			options.Messages = []execution.WebSocketMessage{{Encoding: messageEncoding, Data: *data}}
		}
		err = runWebSocket(ctx, flag.Arg(0), headers, *wsProtocols, options, *timeout, os.Stdout)
	} else if *network == "graphql" {
		document := execution.GraphQLDocument{
			Query: *data, Variables: *graphqlVariables, OperationName: *graphqlOperation, Subscription: *graphqlSubscribe,
		}
		err = runGraphQL(ctx, flag.Arg(0), document, headers, *timeout, os.Stdout, os.Stderr)
	} else if *network == "jsonrpc" || *network == "soap" {
		target := flag.Arg(0)
		var payload execution.HTTPPayload
		if *network == "jsonrpc" {
			target, payload, err = execution.PrepareJSONRPC(target, *rpcMethod, json.RawMessage(*data), json.RawMessage(*rpcID), headers)
		} else {
			payload, err = execution.SOAPPayload(*soapVersion, *soapAction, *data, headers)
		}
		if err == nil {
			request := payload.Request(target)
			request.Timeout, request.FollowRedirects = *timeout, *follow
			err = run(ctx, request, os.Stdout, os.Stderr)
		}
	} else if *network == "grpc" {
		request := execution.GRPCRequest{
			Target: flag.Arg(0), Service: *grpcService, Method: *grpcMethod, Mode: *grpcMode,
			MessageBase64: *data, Timeout: *timeout,
		}
		if *grpcJSON {
			request.MessageBase64, request.MessageJSON = "", *data
		}
		err = runGRPC(ctx, request, *grpcDescriptor, *grpcJSON, os.Stdout, os.Stderr)
	} else if *network == "redis" {
		err = runRedis(ctx, flag.Arg(0), *data, *redisPipeline, *timeout, execution.RedisOptions{
			Protocol: *redisProtocol, Username: *redisUsername, Password: password, Database: *redisDatabase,
			PasswordRef: *redisPasswordRef,
			TLS:         *useTLS, ServerName: *serverName,
		}, *caFile, os.Stdout)
	} else if *network == "tcp" || *network == "udp" {
		var response []byte
		var roots *x509.CertPool
		if *caFile != "" {
			var pem []byte
			pem, err = execution.ReadCAFile(*caFile)
			if err != nil || len(pem) > 1<<20 {
				err = fmt.Errorf("could not read CA file within 1 MiB")
			} else {
				roots, _ = x509.SystemCertPool()
				if roots == nil {
					roots = x509.NewCertPool()
				}
				if !roots.AppendCertsFromPEM(pem) {
					err = fmt.Errorf("CA file contains no usable certificates")
				}
			}
		}
		if err == nil {
			response, err = execution.ExchangeSocket(ctx, execution.SocketRequest{Network: *network, Target: flag.Arg(0), Data: *data, Encoding: *encoding, Timeout: *timeout, Framing: *framing, Delimiter: *delimiter, FixedLength: *length, TLS: *useTLS, ServerName: *serverName, RootCAs: roots, SendCount: *count, Interval: *interval})
		}
		if err == nil {
			_, err = os.Stdout.Write(response)
		}
	} else if *network != "http" {
		err = fmt.Errorf("select http, sse, websocket, tcp, udp, redis, grpc, graphql, jsonrpc, soap, sql, mqtt, amqp, or kafka")
	} else {
		request := execution.HTTPRequest{
			Method: *method, Target: flag.Arg(0), Timeout: *timeout, FollowRedirects: *follow,
			Headers: headers, Body: *data,
		}
		if *responsePlugin != "" {
			err = runResponsePlugin(ctx, request, *responsePlugin, os.Getenv("PROTOVOY_PLUGIN_TRUSTED_KEYS"), os.Stdout, os.Stderr)
		} else {
			err = runScriptedHTTP(ctx, request, *preScript, *postScript, os.Stdout, os.Stderr)
		}
	}
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseCLIHeaders(values []string) ([][2]string, error) {
	var headers [][2]string
	for index, value := range values {
		if strings.ContainsAny(value, "\r\n") {
			return nil, fmt.Errorf("-header %d must contain only one header line", index+1)
		}
		parsed, err := execution.ParseHeaders(value)
		if err != nil || len(parsed) != 1 {
			return nil, fmt.Errorf("-header %d needs a name and a colon", index+1)
		}
		headers = append(headers, parsed[0])
	}
	return headers, nil
}

func runGRPC(ctx context.Context, request execution.GRPCRequest, descriptorPath string, useJSON bool, output, statusOutput io.Writer) error {
	if useJSON && strings.TrimSpace(request.MessageJSON) == "" {
		return fmt.Errorf("-grpc-json requires a JSON payload in -data")
	}
	if descriptorPath != "" {
		file, err := os.Open(descriptorPath)
		if err != nil {
			return fmt.Errorf("could not open the gRPC descriptor file")
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, execution.GRPCDescriptorLimit+1))
		if err != nil || len(data) == 0 || len(data) > execution.GRPCDescriptorLimit {
			return fmt.Errorf("could not read a gRPC descriptor file within 512 KiB")
		}
		request.DescriptorSetBase64 = base64.StdEncoding.EncodeToString(data)
	}
	encoder := json.NewEncoder(output)
	return (execution.GRPCClient{}).Execute(ctx, request, func(status int, _ http.Header) error {
		_, err := fmt.Fprintf(statusOutput, "HTTP %d\n", status)
		return err
	}, func(message []byte, jsonPreview string) error {
		value := struct {
			MessageBase64 string          `json:"messageBase64"`
			MessageJSON   json.RawMessage `json:"messageJson,omitempty"`
		}{base64.StdEncoding.EncodeToString(message), json.RawMessage(jsonPreview)}
		return encoder.Encode(value)
	})
}

func runRedis(ctx context.Context, target, input string, pipeline bool, timeout time.Duration, options execution.RedisOptions, caFile string, output io.Writer) error {
	var commands [][]string
	if pipeline {
		if json.Unmarshal([]byte(input), &commands) != nil {
			return fmt.Errorf("Redis pipeline -data must be a JSON array of command argument arrays")
		}
	} else {
		var arguments []string
		if json.Unmarshal([]byte(input), &arguments) != nil {
			return fmt.Errorf("Redis -data must be a JSON array of command arguments")
		}
		commands = [][]string{arguments}
	}
	if caFile != "" {
		pem, err := execution.ReadCAFile(caFile)
		if err != nil {
			return err
		}
		if len(pem) == 0 {
			return fmt.Errorf("CA file is empty")
		}
		options.CAPEM = string(pem)
	}
	values, err := executeRedis(ctx, target, commands, timeout, options)
	if err != nil {
		return err
	}
	if pipeline {
		return json.NewEncoder(output).Encode(values)
	}
	return json.NewEncoder(output).Encode(values[0])
}

func run(ctx context.Context, request execution.HTTPRequest, output, status io.Writer) error {
	response, err := (execution.HTTPClient{}).Execute(ctx, request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	fmt.Fprintf(status, "HTTP %d\n", response.StatusCode)
	if _, err := io.Copy(output, response.Body); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("could not stream HTTP response")
	}
	return nil
}
