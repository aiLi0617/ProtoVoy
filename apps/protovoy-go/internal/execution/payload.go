package execution

import (
	"fmt"
	"strings"
	"time"
)

// HTTPPayload uses the same method, header-pair and text body fields as
// ProtoVoy's existing HTTP payload. The host supplies the execution target.
type HTTPPayload struct {
	Method          string          `json:"method"`
	Headers         [][2]string     `json:"headers"`
	Body            string          `json:"body"`
	BodyEncoding    string          `json:"bodyEncoding,omitempty"`
	Multipart       []MultipartPart `json:"multipart,omitempty"`
	EventStream     bool            `json:"eventStream,omitempty"`
	FollowRedirects bool            `json:"followRedirects,omitempty"`
	TimeoutMS       int             `json:"timeoutMs,omitempty"`
}

// PreviewRequest may assert a target selected from a project. The execution
// server uses its own configured target and rejects a mismatch.
type PreviewRequest struct {
	HTTPPayload
	ExpectedTarget string            `json:"expectedTarget,omitempty"`
	PreScripts     []string          `json:"preScripts,omitempty"`
	PostScripts    []string          `json:"postScripts,omitempty"`
	Variables      map[string]string `json:"variables,omitempty"`
}

func (payload HTTPPayload) Request(target string) HTTPRequest {
	return HTTPRequest{Target: target, Method: payload.Method, Headers: payload.Headers, Body: payload.Body,
		EventStream: payload.EventStream, FollowRedirects: payload.FollowRedirects, Timeout: time.Duration(payload.TimeoutMS) * time.Millisecond}
}

const DefaultRequestTimeoutMS = 30_000
const MaxRequestTimeoutMS = 300_000

func ValidRequestTimeout(ms int) bool {
	return ms >= 0 && ms <= MaxRequestTimeoutMS
}

func RequestTimeout(ms int) time.Duration {
	if ms == 0 {
		ms = DefaultRequestTimeoutMS
	}
	return time.Duration(ms) * time.Millisecond
}

// ParseHeaders accepts the same one-header-per-line format shown by the
// workbench and preserves repeated header names and their order.
func ParseHeaders(input string) ([][2]string, error) {
	var headers [][2]string
	for index, line := range strings.Split(input, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		name, value, found := strings.Cut(line, ":")
		name = strings.TrimSpace(name)
		if !found || name == "" {
			return nil, fmt.Errorf("header line %d needs a name and a colon", index+1)
		}
		headers = append(headers, [2]string{name, strings.TrimSpace(value)})
	}
	return headers, nil
}
