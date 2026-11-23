package execution

import (
	"bufio"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxSSEEventBytes = 64 << 10

type SSEOptions struct {
	Reconnects       int  `json:"reconnects,omitempty"`
	ReconnectDelayMS *int `json:"reconnectDelayMs,omitempty"`
}

func (options SSEOptions) Valid() bool {
	return options.Reconnects >= 0 && options.Reconnects <= 10 &&
		(options.ReconnectDelayMS == nil || *options.ReconnectDelayMS >= 0 && *options.ReconnectDelayMS <= 60_000)
}

func (options SSEOptions) InitialRetryDelay() time.Duration {
	if options.ReconnectDelayMS == nil {
		return time.Second
	}
	return time.Duration(*options.ReconnectDelayMS) * time.Millisecond
}

func ParseSSEOptions(attempts, delay string) (SSEOptions, error) {
	var options SSEOptions
	var err error
	if attempts = strings.TrimSpace(attempts); attempts != "" {
		options.Reconnects, err = strconv.Atoi(attempts)
		if err != nil || options.Reconnects < 0 || options.Reconnects > 10 {
			return SSEOptions{}, errors.New("enter between 0 and 10 SSE reconnect attempts")
		}
	}
	if delay = strings.TrimSpace(delay); delay != "" {
		ms, err := strconv.Atoi(delay)
		if err != nil || ms < 0 || ms > 60_000 {
			return SSEOptions{}, errors.New("enter an SSE reconnect delay of 0 to 60000 ms")
		}
		options.ReconnectDelayMS = &ms
	}
	return options, nil
}

func PrepareSSE(target string, headers [][2]string, scopes ...map[string]string) (string, HTTPPayload, error) {
	target, request, err := (HTTPPayload{Method: http.MethodGet, Headers: headers, EventStream: true}).ResolveVariables(target, scopes...)
	if err == nil {
		request.Headers = defaultHeader(request.Headers, "Accept", "text/event-stream")
	}
	return target, request, err
}

func SSELastEventID(headers [][2]string) string {
	for _, header := range headers {
		if strings.EqualFold(header[0], "Last-Event-ID") {
			return header[1]
		}
	}
	return ""
}

func SSEHeadersWithEventID(headers [][2]string, id string) [][2]string {
	result := make([][2]string, 0, len(headers)+1)
	for _, header := range headers {
		if !strings.EqualFold(header[0], "Last-Event-ID") {
			result = append(result, header)
		}
	}
	if id != "" {
		result = append(result, [2]string{"Last-Event-ID", id})
	}
	return result
}

type SSEEvent struct {
	Type string
	ID   string
	Data string
}

type SSEDecoder struct {
	input       *bufio.Reader
	first       bool
	skipLF      bool
	LastEventID string
	RetryDelay  time.Duration
}

func NewSSEDecoder(input io.Reader, lastID string, retryDelay time.Duration) *SSEDecoder {
	return &SSEDecoder{input: bufio.NewReader(input), first: true, LastEventID: lastID, RetryDelay: retryDelay}
}

func IsEventStream(status int, contentType string) bool {
	value, _, err := mime.ParseMediaType(contentType)
	return status == http.StatusOK && err == nil && value == "text/event-stream"
}

func (decoder *SSEDecoder) Next() (SSEEvent, error) {
	var data strings.Builder
	var eventType string
	lastID, remaining := decoder.LastEventID, maxSSEEventBytes
	for {
		line, err := decoder.line()
		if err != nil {
			return SSEEvent{}, err
		}
		if decoder.first {
			line = strings.TrimPrefix(line, "\uFEFF")
			decoder.first = false
		}
		if line == "" {
			decoder.LastEventID = lastID
			if data.Len() != 0 {
				if eventType == "" {
					eventType = "message"
				}
				return SSEEvent{Type: eventType, ID: lastID, Data: strings.TrimSuffix(data.String(), "\n")}, nil
			}
			eventType, remaining = "", maxSSEEventBytes
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		remaining -= len(line) + 1
		if remaining < 0 {
			return SSEEvent{}, errors.New("SSE event exceeds 64 KiB")
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "data":
			data.WriteString(value)
			data.WriteByte('\n')
		case "event":
			eventType = value
		case "id":
			if !strings.ContainsRune(value, 0) {
				lastID = value
			}
		case "retry":
			if value != "" && strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) == -1 {
				ms, err := strconv.ParseUint(value, 10, 64)
				if err != nil {
					ms = 60_000
				}
				decoder.RetryDelay = time.Duration(min(ms, 60_000)) * time.Millisecond
			}
		}
	}
}

func (decoder *SSEDecoder) line() (string, error) {
	var out strings.Builder
	for {
		char, err := decoder.input.ReadByte()
		if err != nil {
			return "", err
		}
		if decoder.skipLF {
			decoder.skipLF = false
			if char == '\n' {
				continue
			}
		}
		switch char {
		case '\r', '\n':
			decoder.skipLF = char == '\r'
			return strings.ToValidUTF8(out.String(), "�"), nil
		default:
			if out.Len() == maxSSEEventBytes {
				return "", errors.New("SSE line exceeds 64 KiB")
			}
			out.WriteByte(char)
		}
	}
}
