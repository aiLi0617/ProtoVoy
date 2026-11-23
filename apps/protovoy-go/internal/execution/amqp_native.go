//go:build !js

package execution

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	amqp "github.com/rabbitmq/amqp091-go"
)

const amqpBodyLimit = 1 << 20

type AMQPRequest struct {
	options AMQPOptions
	broker  AMQPBroker
	body    []byte
}

type AMQPBroker struct {
	uri amqp.URI
	tls *tls.Config
}

func PrepareAMQP(target string, options AMQPOptions, scopes ...map[string]string) (AMQPRequest, error) {
	target, err := options.ResolveVariables(target, scopes...)
	if err != nil {
		return AMQPRequest{}, err
	}
	return PrepareAMQPWithPasswordResolver(target, options, func(string) (string, error) { return os.Getenv(AMQPPasswordEnv), nil })
}

func PrepareAMQPWithPasswordResolver(target string, options AMQPOptions, resolve func(string) (string, error)) (AMQPRequest, error) {
	if err := resolveAMQPPassword(target, &options, resolve); err != nil {
		return AMQPRequest{}, err
	}
	broker, err := NewAMQPBroker(target, options.Username, options.Password, options.ServerName, options.CAPEM)
	if err != nil {
		return AMQPRequest{}, err
	}
	return broker.Prepare(options)
}

func NewAMQPBroker(target, username, password, serverName, caPEM string) (AMQPBroker, error) {
	parsed, err := url.Parse(target)
	if err != nil || (parsed.Scheme != "amqp" && parsed.Scheme != "amqps") || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || strings.Contains(target, "#") {
		return AMQPBroker{}, errors.New("enter an AMQP or AMQPS endpoint without URL credentials or options")
	}
	broker, err := amqp.ParseURI(target)
	if err != nil || broker.Port < 1 || broker.Port > 65535 {
		return AMQPBroker{}, errors.New("invalid AMQP broker endpoint")
	}
	if username != "" || password != "" {
		if username == "" || strings.ContainsRune(username, 0) || strings.ContainsRune(password, 0) ||
			len(username)+len(password) > 2048 {
			return AMQPBroker{}, errors.New("invalid AMQP credentials")
		}
		broker.Username, broker.Password = username, password
	}
	if !validAMQPShortString(broker.Vhost) {
		return AMQPBroker{}, errors.New("invalid AMQP virtual host")
	}
	var config *tls.Config
	if parsed.Scheme == "amqps" {
		config = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: serverName}
		if caPEM != "" {
			config.RootCAs, err = socketRoots(caPEM, nil)
			if err != nil {
				return AMQPBroker{}, err
			}
		}
	} else if caPEM != "" || serverName != "" {
		return AMQPBroker{}, errors.New("use amqps:// to configure the server name or CA")
	}
	return AMQPBroker{uri: broker, tls: config}, nil
}

func validAMQPShortString(field string) bool {
	return len(field) <= 255 && !strings.ContainsRune(field, 0) && utf8.ValidString(field)
}

// Prepare validates literal request fields against an already selected broker.
func (broker AMQPBroker) Prepare(options AMQPOptions) (AMQPRequest, error) {
	if options.PasswordRef != "" {
		return AMQPRequest{}, errors.New("resolve AMQP password references before preparing the broker")
	}
	if options.Mode != "publish" && options.Mode != "consume" ||
		options.Mode == "consume" && options.Queue == "" || options.ReceiveLimit < 1 || options.ReceiveLimit > 10_000 {
		return AMQPRequest{}, errors.New("invalid AMQP mode, queue or receive limit")
	}
	switch options.ExchangeType {
	case "direct", "fanout", "topic", "headers":
	default:
		return AMQPRequest{}, errors.New("AMQP exchange type must be direct, fanout, topic or headers")
	}
	for _, field := range []string{options.Exchange, options.RoutingKey, options.Queue, options.ContentType} {
		if !validAMQPShortString(field) {
			return AMQPRequest{}, errors.New("invalid AMQP exchange, routing key, queue or content type")
		}
	}
	if len(options.Payload) > base64.StdEncoding.EncodedLen(amqpBodyLimit) {
		return AMQPRequest{}, errors.New("AMQP payload exceeds 1 MiB")
	}
	var body []byte
	var err error
	switch options.Encoding {
	case "", "text":
		body = []byte(options.Payload)
	case "base64":
		body, err = base64.StdEncoding.DecodeString(options.Payload)
		if err != nil {
			return AMQPRequest{}, errors.New("invalid Base64 AMQP payload")
		}
	default:
		return AMQPRequest{}, errors.New("select text or Base64 AMQP payload encoding")
	}
	if len(body) > amqpBodyLimit {
		return AMQPRequest{}, errors.New("AMQP payload exceeds 1 MiB")
	}
	options.Password, options.Payload, options.CAPEM = "", "", ""
	return AMQPRequest{options: options, broker: broker, body: body}, nil
}

func (request AMQPRequest) Execute(ctx context.Context, connected func() error, emit func(any) error) (err error) {
	defer func() {
		if err != nil && ctx.Err() != nil {
			err = ctx.Err()
		}
	}()
	dial := (&net.Dialer{}).DialContext
	if request.broker.tls != nil {
		dial = (&tls.Dialer{Config: request.broker.tls}).DialContext
	}
	uri := request.broker.uri
	addr := net.JoinHostPort(uri.Host, strconv.Itoa(uri.Port))
	conn, err := dial(ctx, "tcp", addr)
	if err != nil {
		return errors.New("AMQP connection failed")
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	connection, err := amqp.Open(&amqpBoundedConn{Conn: conn, remaining: 32 << 20}, amqp.Config{
		SASL: []amqp.Authentication{uri.PlainAuth()}, Vhost: uri.Vhost,
		ChannelMax: 1, FrameSize: amqpFrameLimit, Heartbeat: 10 * time.Second, Locale: "en_US",
	})
	if err != nil {
		return errors.New("AMQP handshake failed")
	}
	channel, err := connection.Channel()
	if err != nil {
		return errors.New("could not open AMQP channel")
	}
	options := request.options
	if options.Declare && options.Exchange != "" {
		if err := channel.ExchangeDeclare(options.Exchange, options.ExchangeType, options.Durable, false, false, false, nil); err != nil {
			return errors.New("AMQP exchange declaration failed")
		}
	}
	if err := connected(); err != nil {
		return err
	}
	if options.Mode == "publish" {
		err = request.publishAMQP(ctx, channel, emit)
	} else {
		err = request.consumeAMQP(ctx, channel, emit)
	}
	if err == nil {
		_ = connection.CloseDeadline(time.Now().Add(250 * time.Millisecond))
	}
	return err
}

func (request AMQPRequest) publishAMQP(ctx context.Context, channel *amqp.Channel, emit func(any) error) error {
	options := request.options
	if err := channel.Confirm(false); err != nil {
		return errors.New("could not enable AMQP publisher confirms")
	}
	returned := channel.NotifyReturn(make(chan amqp.Return, 1))
	confirmed := channel.NotifyPublish(make(chan amqp.Confirmation, 1))
	message := amqp.Publishing{Body: request.body, ContentType: options.ContentType, DeliveryMode: amqp.Transient}
	if options.Durable {
		message.DeliveryMode = amqp.Persistent
	}
	if err := channel.PublishWithContext(ctx, options.Exchange, options.RoutingKey, true, false, message); err != nil {
		return errors.New("AMQP publish failed")
	}
	var acked bool
	select {
	case confirmation, ok := <-confirmed:
		if !ok {
			return errors.New("AMQP channel closed before publish confirmation")
		}
		acked = confirmation.Ack
	case <-ctx.Done():
		return ctx.Err()
	}
	result := map[string]any{"published": acked, "exchange": options.Exchange, "routingKey": options.RoutingKey,
		"bytes": len(request.body), "confirmation": "nack"}
	if acked {
		result["confirmation"] = "ack"
	}
	select {
	case reply, ok := <-returned:
		if ok {
			result["published"], result["confirmation"], result["replyCode"] = false, "returned", reply.ReplyCode
		}
	default:
	}
	return emit(result)
}

func (request AMQPRequest) consumeAMQP(ctx context.Context, channel *amqp.Channel, emit func(any) error) error {
	options := request.options
	if options.Declare {
		if _, err := channel.QueueDeclare(options.Queue, options.Durable, false, false, false, nil); err != nil {
			return errors.New("AMQP queue declaration failed")
		}
		if options.Exchange != "" {
			if err := channel.QueueBind(options.Queue, options.RoutingKey, options.Exchange, false, nil); err != nil {
				return errors.New("AMQP queue binding failed")
			}
		}
	}
	if !options.AutoAck {
		if err := channel.Qos(1, 0, false); err != nil {
			return errors.New("could not configure AMQP prefetch")
		}
	}
	deliveries, err := channel.Consume(options.Queue, "", options.AutoAck, false, false, false, nil)
	if err != nil {
		return errors.New("AMQP consumer setup failed")
	}
	for range options.ReceiveLimit {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case delivery, ok := <-deliveries:
			if !ok {
				return errors.New("AMQP consumer closed")
			}
			var text any
			if utf8.Valid(delivery.Body) {
				text = string(delivery.Body)
			}
			record := map[string]any{"exchange": delivery.Exchange, "routingKey": delivery.RoutingKey,
				"redelivered": delivery.Redelivered, "payload": text, "payloadBase64": base64.StdEncoding.EncodeToString(delivery.Body),
				"properties": map[string]any{"contentType": delivery.ContentType, "deliveryMode": delivery.DeliveryMode,
					"messageId": delivery.MessageId, "correlationId": delivery.CorrelationId}}
			if err := emit(record); err != nil {
				return err
			}
			if !options.AutoAck {
				if err := delivery.Ack(false); err != nil {
					return errors.New("AMQP acknowledgement failed")
				}
			}
		}
	}
	return nil
}
