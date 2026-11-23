//go:build !js

package execution

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	grpcRequestLimit  = 1 << 20
	grpcResponseLimit = 16 << 20
)

type GRPCClient struct {
	Transport http.RoundTripper
}

type GRPCStatusError struct{ Code uint64 }

func (err GRPCStatusError) Error() string { return fmt.Sprintf("gRPC status %d", err.Code) }

var grpcMetadataName = regexp.MustCompile(`^[a-z0-9_.-]+$`)

var grpcTransport = func() *http.Transport {
	protocols := new(http.Protocols)
	protocols.SetHTTP2(true)
	protocols.SetUnencryptedHTTP2(true)
	return &http.Transport{
		Protocols: protocols, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout: 10 * time.Second, IdleConnTimeout: 90 * time.Second,
		MaxIdleConns: 64, MaxResponseHeaderBytes: 32 << 10, DisableCompression: true,
	}
}()

func ParseGRPCEndpoint(input string) (*url.URL, error) {
	target, err := url.Parse(input)
	if err != nil || target.Hostname() == "" || target.User != nil ||
		(target.Scheme != "http" && target.Scheme != "https") || target.RawQuery != "" ||
		target.ForceQuery || strings.Contains(input, "#") ||
		(target.EscapedPath() != "" && target.EscapedPath() != "/") {
		return nil, errors.New("enter an HTTP or HTTPS gRPC endpoint without a path or credentials")
	}
	return target, nil
}

func (client GRPCClient) Execute(ctx context.Context, input GRPCRequest, onMeta func(int, http.Header) error, onMessage func([]byte, string) error) error {
	target, err := ParseGRPCEndpoint(input.Target)
	if err != nil {
		return err
	}
	if err := ValidateGRPCSelection(input.Service, input.Method, input.Mode); err != nil {
		return err
	}
	var multipleRequests, multipleResponses bool
	switch input.Mode {
	case "", "unary":
	case "server_streaming":
		multipleResponses = true
	case "client_streaming":
		multipleRequests = true
	case "bidi_streaming":
		multipleRequests, multipleResponses = true, true
	}
	if !ValidRequestTimeout(input.TimeoutMS) {
		return errors.New("invalid gRPC timeout")
	}
	timeout := input.Timeout
	if timeout <= 0 {
		timeout = RequestTimeout(input.TimeoutMS)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	schema, err := grpcSchemaFor(input)
	if err != nil {
		return err
	}
	if schema == nil && strings.TrimSpace(input.MessageJSON) != "" {
		if len(input.MessageJSON) > grpcRequestLimit || !json.Valid([]byte(input.MessageJSON)) {
			return errors.New("expected valid gRPC request JSON within 1 MiB")
		}
		schema, err = client.reflectGRPCSchema(ctx, *target, input)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
	}
	body, err := encodeGRPCRequest(input, schema, multipleRequests)
	if err != nil {
		return err
	}
	target.Path = "/" + input.Service + "/" + input.Method
	return client.call(ctx, target.String(), body, input.Metadata, multipleResponses, grpcResponseLimit, onMeta, func(data []byte) error {
		return onMessage(data, schema.preview(data))
	})
}

func (client GRPCClient) call(ctx context.Context, target string, body []byte, metadata [][2]string, multipleResponses bool, responseLimit int, onMeta func(int, http.Header) error, onMessage func([]byte) error) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return errors.New("could not prepare gRPC request")
	}
	request.Header.Set("Content-Type", "application/grpc")
	request.Header.Set("TE", "trailers")
	request.Header.Set("Grpc-Accept-Encoding", "identity")
	if deadline, ok := ctx.Deadline(); ok {
		request.Header.Set("Grpc-Timeout", strconv.FormatInt(min(max(1, time.Until(deadline).Milliseconds()), 99_999_999), 10)+"m")
	}
	if err := appendGRPCMetadata(request.Header, metadata); err != nil {
		return err
	}
	transport := client.Transport
	if transport == nil {
		transport = grpcTransport
	}
	httpClient := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	response, err := httpClient.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("gRPC connection failed")
	}
	defer response.Body.Close()
	if onMeta != nil {
		if err := onMeta(response.StatusCode, response.Header); err != nil {
			return err
		}
	}
	contentType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if response.ProtoMajor != 2 || response.StatusCode != http.StatusOK ||
		(contentType != "application/grpc" && contentType != "application/grpc+proto") {
		return errors.New("expected a gRPC response over HTTP/2")
	}
	count, err := readGRPCMessages(response.Body, multipleResponses, responseLimit, onMessage)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	status := response.Trailer.Get("Grpc-Status")
	if status == "" && count == 0 {
		status = response.Header.Get("Grpc-Status")
	}
	code, err := strconv.ParseUint(status, 10, 8)
	if err != nil || code > 16 {
		return errors.New("missing or invalid gRPC status")
	}
	if code != 0 {
		return GRPCStatusError{Code: code}
	}
	if !multipleResponses && count != 1 {
		return errors.New("gRPC call requires one response message")
	}
	return nil
}

func encodeGRPCMessages(input string, multiple bool) ([]byte, error) {
	if len(input) > 2*grpcRequestLimit {
		return nil, errors.New("gRPC request exceeds 1 MiB")
	}
	var body []byte
	lines := []string{input}
	if multiple {
		lines = strings.Split(input, "\n")
	}
	for _, value := range lines {
		value = strings.TrimSpace(value)
		if multiple && value == "" {
			continue
		}
		message, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return nil, errors.New("gRPC message must be Base64 protobuf bytes")
		}
		body, err = appendGRPCFrame(body, message)
		if err != nil {
			return nil, err
		}
	}
	return body, nil
}

func appendGRPCFrame(body, message []byte) ([]byte, error) {
	if len(message)+5 > grpcRequestLimit-len(body) {
		return nil, errors.New("gRPC request exceeds 1 MiB")
	}
	body = append(body, 0)
	body = binary.BigEndian.AppendUint32(body, uint32(len(message)))
	return append(body, message...), nil
}

func readGRPCMessages(body io.Reader, multiple bool, budget int, onMessage func([]byte) error) (int, error) {
	count := 0
	var header [5]byte
	for {
		if _, err := io.ReadFull(body, header[:]); err != nil {
			if err == io.EOF {
				return count, nil
			}
			return count, errors.New("incomplete gRPC response frame")
		}
		length := binary.BigEndian.Uint32(header[1:])
		if header[0] != 0 {
			return count, errors.New("unsupported gRPC compression flag")
		}
		if length > GRPCMessageLimit || budget < 5 || int(length) > budget-5 {
			return count, errors.New("gRPC response exceeds its size limit")
		}
		if !multiple && count != 0 {
			return count, errors.New("gRPC call returned more than one response message")
		}
		budget -= int(length) + 5
		message := make([]byte, int(length))
		if _, err := io.ReadFull(body, message); err != nil {
			return count, errors.New("incomplete gRPC response message")
		}
		if err := onMessage(message); err != nil {
			return count, err
		}
		count++
	}
}

func appendGRPCMetadata(headers http.Header, pairs [][2]string) error {
	if len(pairs) > 128 {
		return errors.New("too many gRPC metadata headers")
	}
	for _, pair := range pairs {
		key, value := strings.ToLower(pair[0]), pair[1]
		if !grpcMetadataName.MatchString(key) || strings.HasPrefix(key, "grpc-") ||
			strings.ContainsFunc(value, func(c rune) bool { return c < 32 || c > 126 }) {
			return errors.New("invalid or reserved gRPC metadata")
		}
		switch key {
		case "te", "content-type", "content-length", "host", "connection", "transfer-encoding", "upgrade", "trailer", "accept-encoding", "proxy-authorization":
			return errors.New("invalid or reserved gRPC metadata")
		}
		if strings.HasSuffix(key, "-bin") {
			data, err := base64.StdEncoding.DecodeString(value)
			if err != nil {
				data, err = base64.RawStdEncoding.DecodeString(value)
			}
			if err != nil {
				return errors.New("binary gRPC metadata must be Base64")
			}
			value = base64.RawStdEncoding.EncodeToString(data)
		}
		headers.Add(key, value)
	}
	return nil
}
