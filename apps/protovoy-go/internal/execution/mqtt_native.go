//go:build !js

package execution

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const mqttPacketLimit = 1 << 20

type MQTTRequest struct {
	options MQTTOptions
	address string
	tls     *tls.Config
	connect []byte
	payload []byte
}

type MQTTBroker struct {
	address string
	tls     *tls.Config
}

func PrepareMQTT(target string, options MQTTOptions, scopes ...map[string]string) (MQTTRequest, error) {
	target, err := options.ResolveVariables(target, scopes...)
	if err != nil {
		return MQTTRequest{}, err
	}
	return PrepareMQTTWithPasswordResolver(target, options, func(string) (string, error) { return os.Getenv(MQTTPasswordEnv), nil })
}

// PrepareMQTTWithPasswordResolver accepts literal options after variable expansion.
func PrepareMQTTWithPasswordResolver(target string, options MQTTOptions, resolve func(string) (string, error)) (MQTTRequest, error) {
	if err := resolveMQTTPassword(target, &options, resolve); err != nil {
		return MQTTRequest{}, err
	}
	broker, err := NewMQTTBroker(target, options.ServerName, options.CAPEM)
	if err != nil {
		return MQTTRequest{}, err
	}
	return broker.Prepare(options)
}

func NewMQTTBroker(target, serverName, caPEM string) (MQTTBroker, error) {
	if !strings.Contains(target, "://") {
		target = "mqtt://" + target
	}
	u, err := url.Parse(target)
	if err != nil || (u.Scheme != "mqtt" && u.Scheme != "mqtts") || u.Hostname() == "" || u.User != nil ||
		(u.EscapedPath() != "" && u.EscapedPath() != "/") || u.RawQuery != "" || u.ForceQuery || strings.Contains(target, "#") {
		return MQTTBroker{}, errors.New("enter an MQTT host, mqtt:// or mqtts:// endpoint without credentials")
	}
	port := u.Port()
	if port == "" {
		port = "1883"
		if u.Scheme == "mqtts" {
			port = "8883"
		}
	}
	if value, err := strconv.ParseUint(port, 10, 16); err != nil || value == 0 {
		return MQTTBroker{}, errors.New("invalid MQTT port")
	}
	broker := MQTTBroker{address: net.JoinHostPort(u.Hostname(), port)}
	if u.Scheme == "mqtts" {
		broker.tls = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: serverName}
		if caPEM != "" {
			broker.tls.RootCAs, err = socketRoots(caPEM, nil)
			if err != nil {
				return MQTTBroker{}, err
			}
		}
	} else if serverName != "" || caPEM != "" {
		return MQTTBroker{}, errors.New("use mqtts:// to configure the server name or CA")
	}
	return broker, nil
}

// Prepare validates literal request fields for an already selected broker.
func (broker MQTTBroker) Prepare(options MQTTOptions) (MQTTRequest, error) {
	if options.PasswordRef != "" {
		return MQTTRequest{}, errors.New("resolve MQTT password references before preparing the broker")
	}
	request := MQTTRequest{address: broker.address, tls: broker.tls}
	var err error
	if options.Mode != "publish" && options.Mode != "subscribe" || options.QoS < 0 || options.QoS > 2 ||
		options.KeepAlive < 0 || options.KeepAlive > 65535 || options.ReceiveLimit < 1 || options.ReceiveLimit > 10_000 {
		return MQTTRequest{}, errors.New("invalid MQTT mode, QoS, keep alive or receive limit")
	}
	if !mqttTopic(options.Topic, options.Mode == "subscribe") {
		return MQTTRequest{}, errors.New("invalid MQTT topic or topic filter")
	}
	if options.ClientID == "" {
		options.ClientID = "protovoy-client"
	}
	if !mqttText(options.ClientID) || !mqttText(options.Username) || len(options.Password) > 65535 ||
		options.Password != "" && options.Username == "" {
		return MQTTRequest{}, errors.New("invalid MQTT client ID or credentials")
	}
	if len(options.Payload) > base64.StdEncoding.EncodedLen(mqttPacketLimit) {
		return MQTTRequest{}, errors.New("MQTT payload exceeds 1 MiB")
	}
	switch options.Encoding {
	case "", "text":
		request.payload = []byte(options.Payload)
	case "base64":
		request.payload, err = base64.StdEncoding.DecodeString(options.Payload)
		if err != nil {
			return MQTTRequest{}, errors.New("invalid Base64 MQTT payload")
		}
	default:
		return MQTTRequest{}, errors.New("select text or Base64 MQTT payload encoding")
	}
	if len(request.payload)+len(options.Topic)+4 > mqttPacketLimit {
		return MQTTRequest{}, errors.New("MQTT packet exceeds 1 MiB")
	}
	request.connect = []byte{0, 4, 'M', 'Q', 'T', 'T', 4, 0}
	if options.CleanSession {
		request.connect[7] |= 2
	}
	request.connect = binary.BigEndian.AppendUint16(request.connect, uint16(options.KeepAlive))
	request.connect = mqttAppendString(request.connect, options.ClientID)
	if options.Username != "" {
		request.connect[7] |= 0x80
		request.connect = mqttAppendString(request.connect, options.Username)
	}
	if options.Password != "" {
		request.connect[7] |= 0x40
		request.connect = mqttAppendString(request.connect, options.Password)
	}
	options.Password, options.Payload, options.CAPEM = "", "", ""
	request.options = options
	return request, nil
}

func mqttText(value string) bool {
	return len(value) <= 65535 && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

func mqttTopic(value string, filter bool) bool {
	if value == "" || !mqttText(value) {
		return false
	}
	if !filter {
		return !strings.ContainsAny(value, "+#")
	}
	for prefix, rest, more := strings.Cut(value, "/"); ; prefix, rest, more = strings.Cut(rest, "/") {
		if strings.ContainsAny(prefix, "+#") && prefix != "+" && (prefix != "#" || more) {
			return false
		}
		if !more {
			return true
		}
	}
}

func mqttAppendString(body []byte, value string) []byte {
	return append(binary.BigEndian.AppendUint16(body, uint16(len(value))), value...)
}

type mqttWire struct {
	conn   net.Conn
	read   *bufio.Reader
	mu     sync.Mutex
	closed bool
}

func (wire *mqttWire) send(header byte, body []byte) error {
	frame := []byte{header}
	length := len(body)
	for length >= 128 {
		frame = append(frame, byte(length%128)|128)
		length /= 128
	}
	frame = append(frame, byte(length))
	wire.mu.Lock()
	defer wire.mu.Unlock()
	if wire.closed {
		return nil
	}
	wire.closed = header == 0xe0
	if _, err := wire.conn.Write(append(frame, body...)); err != nil {
		return errors.New("could not write MQTT packet")
	}
	return nil
}

func (wire *mqttWire) receive() (byte, []byte, error) {
	header, err := wire.read.ReadByte()
	if err != nil {
		return 0, nil, errors.New("MQTT connection closed while reading")
	}
	length := 0
	for shift := range 4 {
		value, err := wire.read.ReadByte()
		if err != nil {
			break
		}
		length |= int(value&127) << (7 * shift)
		if length > mqttPacketLimit {
			return 0, nil, errors.New("MQTT packet exceeds 1 MiB")
		}
		if value < 128 && (shift == 0 || value != 0) {
			body := make([]byte, length)
			if _, err := io.ReadFull(wire.read, body); err != nil {
				return 0, nil, errors.New("incomplete MQTT packet")
			}
			return header, body, nil
		}
		if value < 128 {
			break
		}
	}
	return 0, nil, errors.New("invalid MQTT packet length")
}

func (request MQTTRequest) Execute(ctx context.Context, connected func() error, emit func(any) error) (err error) {
	defer func() {
		if err != nil && ctx.Err() != nil {
			err = ctx.Err()
		}
	}()
	dial := (&net.Dialer{}).DialContext
	if request.tls != nil {
		dial = (&tls.Dialer{Config: request.tls}).DialContext
	}
	conn, err := dial(ctx, "tcp", request.address)
	if err != nil {
		return errors.New("MQTT connection failed")
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return errors.New("could not set MQTT deadline")
		}
	}
	wire := &mqttWire{conn: conn, read: bufio.NewReader(conn)}
	if err := wire.send(0x10, request.connect); err != nil {
		return err
	}
	header, ack, err := wire.receive()
	if err != nil {
		return err
	}
	if header != 0x20 || len(ack) != 2 || ack[0] > 1 || request.options.CleanSession && ack[0] != 0 {
		return errors.New("invalid MQTT CONNACK")
	}
	if ack[1] != 0 {
		return fmt.Errorf("MQTT connection refused with code %d", ack[1])
	}
	if err := connected(); err != nil {
		return err
	}
	pings, stopPings := context.WithCancel(ctx)
	defer stopPings()
	if request.options.KeepAlive > 0 {
		go wire.keepAlive(pings, time.Duration(request.options.KeepAlive)*time.Second/2)
	}
	if request.options.Mode == "publish" {
		err = request.publish(wire, emit)
	} else {
		err = request.subscribe(wire, emit)
	}
	if err == nil {
		_ = wire.send(0xe0, nil)
	}
	return err
}

func (wire *mqttWire) keepAlive(ctx context.Context, interval time.Duration) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if err := wire.send(0xc0, nil); err != nil {
				_ = wire.conn.Close()
				return
			}
		}
	}
}

func (wire *mqttWire) nextPacket() (byte, []byte, error) {
	for {
		header, data, err := wire.receive()
		if err != nil || header != 0xd0 || len(data) != 0 {
			return header, data, err
		}
	}
}

func (request MQTTRequest) publish(wire *mqttWire, emit func(any) error) error {
	options := request.options
	header := 0x30 | byte(options.QoS)<<1
	if options.Retain {
		header |= 1
	}
	body := mqttAppendString(nil, options.Topic)
	id := []byte{0, 1}
	if options.QoS > 0 {
		body = append(body, id...)
	}
	if err := wire.send(header, append(body, request.payload...)); err != nil {
		return err
	}
	waitAck := func(expected byte) error {
		header, body, err := wire.nextPacket()
		if err != nil {
			return err
		}
		if header != expected || !bytes.Equal(body, id) {
			return errors.New("unexpected MQTT publish acknowledgement")
		}
		return nil
	}
	switch options.QoS {
	case 1:
		if err := waitAck(0x40); err != nil {
			return err
		}
	case 2:
		if err := waitAck(0x50); err != nil {
			return err
		}
		if err := wire.send(0x62, id); err != nil {
			return err
		}
		if err := waitAck(0x70); err != nil {
			return err
		}
	}
	return emit(map[string]any{"published": true, "topic": options.Topic, "qos": options.QoS,
		"bytes": len(request.payload), "retained": options.Retain})
}

type mqttMessage struct {
	topic  string
	data   []byte
	qos    byte
	retain bool
}

func parseMQTTPublish(header byte, body []byte) (mqttMessage, uint16, error) {
	message := mqttMessage{qos: (header >> 1) & 3, retain: header&1 != 0}
	invalid := errors.New("invalid MQTT PUBLISH packet")
	if len(body) < 2 || message.qos == 3 || (message.qos == 0 && header&8 != 0) {
		return message, 0, invalid
	}
	size := int(binary.BigEndian.Uint16(body))
	if size > len(body)-2 {
		return message, 0, invalid
	}
	message.topic = string(body[2 : 2+size])
	if !mqttTopic(message.topic, false) {
		return message, 0, invalid
	}
	message.data = body[2+size:]
	var id uint16
	if message.qos != 0 {
		if len(message.data) < 2 {
			return message, 0, invalid
		}
		id = binary.BigEndian.Uint16(message.data)
		if id == 0 {
			return message, 0, invalid
		}
		message.data = message.data[2:]
	}
	return message, id, nil
}

func (message mqttMessage) event() map[string]any {
	var text any
	if utf8.Valid(message.data) {
		text = string(message.data)
	}
	return map[string]any{"topic": message.topic, "qos": message.qos, "retain": message.retain,
		"payload": text, "payloadBase64": base64.StdEncoding.EncodeToString(message.data)}
}

func (request MQTTRequest) subscribe(wire *mqttWire, emit func(any) error) error {
	options := request.options
	body := mqttAppendString([]byte{0, 1}, options.Topic)
	if err := wire.send(0x82, append(body, byte(options.QoS))); err != nil {
		return err
	}
	pending := make(map[uint16]mqttMessage)
	pendingBytes, received := 0, 0
	acknowledged := false
	deliver := func(message mqttMessage) error {
		if received == options.ReceiveLimit {
			return nil
		}
		received++
		return emit(message.event())
	}
	for !acknowledged || received < options.ReceiveLimit {
		header, body, err := wire.nextPacket()
		if err != nil {
			return err
		}
		switch {
		case header == 0x90:
			if acknowledged || len(body) != 3 || body[0] != 0 || body[1] != 1 || body[2] > byte(options.QoS) {
				return errors.New("MQTT subscription was rejected or returned an invalid SUBACK")
			}
			acknowledged = true
		case header>>4 == 3:
			message, id, err := parseMQTTPublish(header, body)
			if err != nil {
				return err
			}
			switch message.qos {
			case 0, 1:
				if message.qos == 1 {
					if err := wire.send(0x40, binary.BigEndian.AppendUint16(nil, id)); err != nil {
						return err
					}
				}
				if err := deliver(message); err != nil {
					return err
				}
			case 2:
				if _, exists := pending[id]; !exists {
					pendingBytes += len(message.data) + len(message.topic)
					if len(pending) >= 128 || pendingBytes > 2*mqttPacketLimit {
						return errors.New("too many pending MQTT QoS 2 messages")
					}
					pending[id] = message
				}
				if err := wire.send(0x50, binary.BigEndian.AppendUint16(nil, id)); err != nil {
					return err
				}
			}
		case header == 0x62:
			if len(body) != 2 || binary.BigEndian.Uint16(body) == 0 {
				return errors.New("invalid MQTT PUBREL packet")
			}
			if err := wire.send(0x70, body); err != nil {
				return err
			}
			id := binary.BigEndian.Uint16(body)
			if message, exists := pending[id]; exists {
				delete(pending, id)
				pendingBytes -= len(message.data) + len(message.topic)
				if err := deliver(message); err != nil {
					return err
				}
			}
		default:
			return errors.New("unexpected MQTT subscription packet")
		}
	}
	return nil
}
