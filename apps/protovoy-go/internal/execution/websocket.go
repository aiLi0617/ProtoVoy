package execution

import (
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"time"
)

const (
	WebSocketMessageLimit      = 256 << 10
	WebSocketSetupLimit        = 64 << 10
	WebSocketBridgePath        = "/api/websocket"
	AgentWebSocketTicketPath   = "/v1/websocket-ticket"
	AgentWebSocketPath         = "/v1/websocket"
	AgentWebSocketProtocol     = "protovoy"
	AgentWebSocketTicketPrefix = "protovoy-ticket."
)

type WebSocketOptions struct {
	Subprotocols string `json:"subprotocols,omitempty"`
	WebSocketAutomation
}

type WebSocketAutomation struct {
	Messages         []WebSocketMessage `json:"messages,omitempty"`
	ReceiveLimit     *int               `json:"receiveLimit,omitempty"`
	ReconnectMax     int                `json:"reconnectMax,omitempty"`
	ReconnectDelayMS *int               `json:"reconnectDelayMs,omitempty"`
	IdleTimeoutMS    int                `json:"idleTimeoutMs,omitempty"`
}

type WebSocketMessage struct {
	Encoding string `json:"encoding,omitempty"`
	Data     string `json:"data"`
}

func (options WebSocketAutomation) ReconnectDelay() time.Duration {
	if options.ReconnectDelayMS == nil {
		return time.Second
	}
	return time.Duration(*options.ReconnectDelayMS) * time.Millisecond
}

func (options WebSocketAutomation) Frames(scopes ...map[string]string) ([][]byte, error) {
	if len(options.Messages) > 64 || options.ReceiveLimit != nil && (*options.ReceiveLimit < 0 || *options.ReceiveLimit > 10_000) ||
		options.ReconnectMax < 0 || options.ReconnectMax > 10 ||
		options.ReconnectDelayMS != nil && (*options.ReconnectDelayMS < 0 || *options.ReconnectDelayMS > 60_000) ||
		options.IdleTimeoutMS < 0 || options.IdleTimeoutMS > 300_000 {
		return nil, errors.New("invalid WebSocket message count, receive limit, reconnect or timeout settings")
	}
	resolver := newVariableResolver(scopes)
	frames := make([][]byte, 0, len(options.Messages))
	total := 0
	for _, message := range options.Messages {
		var data []byte
		switch message.Encoding {
		case "", "text":
			text := message.Data
			if scopes != nil {
				text = resolver.text(text)
			}
			data = []byte(text)
		case "binary":
			if len(message.Data) > base64.StdEncoding.EncodedLen(WebSocketMessageLimit) {
				return nil, errors.New("WebSocket binary message exceeds 256 KiB")
			}
			var err error
			data, err = base64.StdEncoding.DecodeString(message.Data)
			if err != nil {
				return nil, errors.New("WebSocket binary message must use Base64")
			}
		default:
			return nil, errors.New("WebSocket message encoding must be text or binary")
		}
		total += len(data)
		if len(data) > WebSocketMessageLimit || total > 1<<20 {
			return nil, errors.New("WebSocket startup messages exceed their size limit")
		}
		frames = append(frames, data)
	}
	return frames, resolver.err
}

type WebSocketSetup struct {
	ExpectedTarget string      `json:"expectedTarget,omitempty"`
	Headers        [][2]string `json:"headers,omitempty"`
	Subprotocols   []string    `json:"subprotocols,omitempty"`
}

type WebSocketReady struct {
	Subprotocol string `json:"subprotocol,omitempty"`
	Error       string `json:"error,omitempty"`
}

func ValidWebSocketTarget(target string) bool {
	parsed, err := url.Parse(target)
	return err == nil && parsed.Hostname() != "" && parsed.User == nil && parsed.Fragment == "" &&
		(parsed.Scheme == "ws" || parsed.Scheme == "wss")
}

func PrepareWebSocket(target string, headers [][2]string, protocols string, scopes ...map[string]string) (WebSocketSetup, error) {
	resolver := newVariableResolver(scopes)
	resolved := resolver.text(target)
	headers = resolver.headers(headers)
	protocols = resolver.text(protocols)
	if resolver.err != nil {
		return WebSocketSetup{}, resolver.err
	}
	if target != "" && !ValidWebSocketTarget(resolved) {
		return WebSocketSetup{}, errors.New("enter a valid ws:// or wss:// URL")
	}
	setup := WebSocketSetup{ExpectedTarget: resolved, Headers: headers}
	if strings.TrimSpace(protocols) != "" {
		for value := range strings.SplitSeq(protocols, ",") {
			setup.Subprotocols = append(setup.Subprotocols, strings.TrimSpace(value))
		}
	}
	return setup, setup.Validate()
}

func (setup WebSocketSetup) Validate() error {
	if len(setup.Headers) > 128 || len(setup.Subprotocols) > 32 {
		return errors.New("too many WebSocket headers or subprotocols")
	}
	for _, header := range setup.Headers {
		name := strings.ToLower(header[0])
		if !validWebSocketToken(name) || strings.ContainsAny(header[1], "\x00\r\n") {
			return errors.New("invalid WebSocket handshake header")
		}
		if name == "host" || name == "connection" || name == "upgrade" || name == "content-length" ||
			name == "transfer-encoding" || strings.HasPrefix(name, "sec-websocket-") || strings.HasPrefix(name, "proxy-") {
			return errors.New("use the handshake fields for reserved WebSocket headers")
		}
	}
	seen := make(map[string]bool, len(setup.Subprotocols))
	for _, protocol := range setup.Subprotocols {
		if !validWebSocketToken(protocol) || seen[protocol] {
			return errors.New("enter unique comma-separated WebSocket subprotocols")
		}
		seen[protocol] = true
	}
	return nil
}

func validWebSocketToken(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' ||
			char >= '0' && char <= '9' || strings.ContainsRune("!#$%&'*+-.^`_|~", char)) {
			return false
		}
	}
	return true
}
