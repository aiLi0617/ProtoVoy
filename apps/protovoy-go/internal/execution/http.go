package execution

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// PreviewLimit caps the response data shown in the request workbench.
const PreviewLimit = 8 << 10

// HTTPRequest contains the input required to execute an HTTP request.
type HTTPRequest struct {
	Method          string
	Target          string
	Headers         [][2]string
	Body            string
	Timeout         time.Duration
	FollowRedirects bool
	EventStream     bool
}

type HTTPResponse struct {
	StatusCode int
	Headers    http.Header
	Body       io.ReadCloser
}

// HTTPClient owns the transport used by the execution host. A server host can
// supply a restricted transport instead of using the local client's defaults.
type HTTPClient struct {
	Transport        http.RoundTripper
	ValidateRedirect func(*http.Request) error
}

var (
	eventStreamTransport     *http.Transport
	eventStreamTransportOnce sync.Once
)

func defaultEventStreamTransport() *http.Transport {
	eventStreamTransportOnce.Do(func() {
		eventStreamTransport = http.DefaultTransport.(*http.Transport).Clone()
		eventStreamTransport.ResponseHeaderTimeout = 30 * time.Second
	})
	return eventStreamTransport
}

// Execute returns the response as soon as headers arrive. Callers must close
// the body and can cancel reads through ctx.
func (client HTTPClient) Execute(ctx context.Context, input HTTPRequest) (*HTTPResponse, error) {
	request, err := http.NewRequestWithContext(ctx, input.Method, input.Target, strings.NewReader(input.Body))
	if err != nil || request.URL.Hostname() == "" || request.URL.User != nil ||
		(request.URL.Scheme != "http" && request.URL.Scheme != "https") {
		return nil, fmt.Errorf("enter a valid HTTP or HTTPS request")
	}
	for _, header := range input.Headers {
		request.Header.Add(header[0], header[1])
	}

	timeout := input.Timeout
	if timeout <= 0 {
		timeout = DefaultRequestTimeoutMS * time.Millisecond
	}
	transport := &http.Client{
		Transport: client.Transport,
		Timeout:   timeout,
	}
	if input.EventStream {
		transport.Timeout = 0
		if transport.Transport == nil {
			transport.Transport = defaultEventStreamTransport()
		}
	}
	if !input.FollowRedirects {
		transport.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	var redirectErr error
	if input.FollowRedirects && client.ValidateRedirect != nil {
		transport.CheckRedirect = func(next *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("too many redirects")
			}
			redirectErr = client.ValidateRedirect(next)
			return redirectErr
		}
	}
	response, err := transport.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if redirectErr != nil {
			return nil, redirectErr
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, context.DeadlineExceeded
		}
		// The transport error may contain URL credentials, query values, or
		// request header values. Leave it out of display and log messages.
		return nil, fmt.Errorf("HTTP request failed")
	}
	return &HTTPResponse{StatusCode: response.StatusCode, Headers: response.Header, Body: response.Body}, nil
}
