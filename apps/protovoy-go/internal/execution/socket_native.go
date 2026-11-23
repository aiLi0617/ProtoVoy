//go:build !js

package execution

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"strings"
	"time"
)

// SocketRequest executes one bounded TCP or UDP exchange. Target is host:port.
type SocketRequest struct {
	Network     string
	Target      string
	Data        string
	Encoding    string
	Timeout     time.Duration
	Framing     string
	Delimiter   string
	FixedLength int
	TLS         bool
	ServerName  string
	RootCAs     *x509.CertPool
	CAPEM       string
	SendCount   int
	Interval    time.Duration
}

func (payload SocketPayload) Request(target string) SocketRequest {
	return SocketRequest{
		Network: payload.Network, Target: target, Data: payload.Data, Encoding: payload.Encoding,
		Framing: payload.Framing, Delimiter: payload.Delimiter, FixedLength: payload.FixedLength,
		TLS: payload.TLS, ServerName: payload.ServerName, CAPEM: payload.CAPEM, Timeout: RequestTimeout(payload.TimeoutMS),
		SendCount: payload.SendCount, Interval: time.Duration(payload.IntervalMS) * time.Millisecond,
	}
}

func ExchangeSocket(ctx context.Context, request SocketRequest) ([]byte, error) {
	count := request.SendCount
	if count == 0 {
		count = 1
	}
	if count < 1 || count > 10000 || request.Interval < 0 {
		return nil, errors.New("use 1 to 10000 sends and a nonnegative interval")
	}
	if request.Network != "tcp" && request.Network != "udp" {
		return nil, errors.New("select TCP or UDP")
	}
	if request.Network != "tcp" && (request.TLS || request.ServerName != "" || request.RootCAs != nil || request.CAPEM != "") {
		return nil, errors.New("TLS is supported for TCP only")
	}
	if !request.TLS && (request.ServerName != "" || request.RootCAs != nil || request.CAPEM != "") {
		return nil, errors.New("enable TLS to configure its server name or CA")
	}
	if request.CAPEM != "" {
		roots, err := socketRoots(request.CAPEM, request.RootCAs)
		if err != nil {
			return nil, err
		}
		request.RootCAs = roots
	}
	const responseLimit = SocketResponseLimit
	switch request.Framing {
	case "", "raw":
	case "fixed":
		if request.FixedLength <= 0 || request.FixedLength > responseLimit {
			return nil, errors.New("fixed response length must be between 1 and 524288")
		}
	case "delimiter":
		if len(request.Delimiter) == 0 || len(request.Delimiter) > 4096 {
			return nil, errors.New("enter a delimiter of 1 to 4096 bytes")
		}
	default:
		return nil, errors.New("select raw, fixed, or delimiter framing")
	}
	if request.Network == "udp" && request.Framing != "" && request.Framing != "raw" {
		return nil, errors.New("UDP responses use datagram boundaries")
	}
	if _, _, err := net.SplitHostPort(request.Target); err != nil {
		return nil, errors.New("enter a host:port socket target")
	}
	if len(request.Data) > 512<<10 {
		return nil, errors.New("socket payload exceeds 512 KiB")
	}
	var data []byte
	var err error
	switch request.Encoding {
	case "", "text":
		data = []byte(request.Data)
	case "base64":
		data, err = base64.StdEncoding.DecodeString(request.Data)
	case "hex":
		data, err = hex.DecodeString(strings.Join(strings.Fields(request.Data), ""))
	default:
		return nil, errors.New("select text, base64, or hex encoding")
	}
	if err != nil {
		return nil, errors.New("invalid encoded socket payload")
	}
	if request.Network == "udp" && len(data) > 65507 {
		return nil, errors.New("UDP payload exceeds 65,507 bytes")
	}
	timeout := request.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var connection net.Conn
	if request.TLS {
		dialer := tls.Dialer{Config: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: request.ServerName, RootCAs: request.RootCAs}}
		connection, err = dialer.DialContext(ctx, "tcp", request.Target)
	} else {
		connection, err = (&net.Dialer{}).DialContext(ctx, request.Network, request.Target)
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("socket connection failed")
	}
	defer connection.Close()
	stop := context.AfterFunc(ctx, func() { connection.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if err := connection.SetDeadline(deadline); err != nil {
		return nil, errors.New("could not set socket timeout")
	}
	for index := 0; index < count; index++ {
		if request.Network == "udp" && len(data) == 0 {
			if _, err := connection.Write(data); err != nil {
				return nil, errors.New("could not send UDP datagram")
			}
		}
		remaining := data
		for len(remaining) > 0 {
			n, writeErr := connection.Write(remaining)
			if writeErr != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				return nil, errors.New("could not send socket payload")
			}
			if n == 0 {
				return nil, errors.New("socket write made no progress")
			}
			remaining = remaining[n:]
		}
		if index+1 < count && request.Interval > 0 {
			timer := time.NewTimer(request.Interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}
	if request.Network == "tcp" && request.Framing == "fixed" {
		buffer := make([]byte, request.FixedLength)
		n, err := io.ReadFull(connection, buffer)
		if ctx.Err() != nil {
			return buffer[:n], ctx.Err()
		}
		if err != nil {
			if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
				return buffer[:n], context.DeadlineExceeded
			}
			return buffer[:n], errors.New("connection ended before the fixed response was complete")
		}
		return buffer, nil
	}
	if request.Network == "tcp" && request.Framing == "delimiter" {
		var response []byte
		chunk := make([]byte, 4096)
		delimiter := []byte(request.Delimiter)
		for {
			start := max(0, len(response)-len(delimiter)+1)
			n, err := connection.Read(chunk[:min(len(chunk), responseLimit-len(response))])
			response = append(response, chunk[:n]...)
			if index := bytes.Index(response[start:], delimiter); index >= 0 {
				return response[:start+index+len(delimiter)], nil
			}
			if ctx.Err() != nil {
				return response, ctx.Err()
			}
			if err != nil {
				if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
					return response, context.DeadlineExceeded
				}
				return response, errors.New("connection ended before the response delimiter")
			}
			if len(response) == responseLimit {
				return response, errors.New("socket response exceeds 512 KiB")
			}
		}
	}
	buffer := make([]byte, 65535)
	n, err := connection.Read(buffer)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil && err != io.EOF {
		if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
			return buffer[:n], context.DeadlineExceeded
		}
		return nil, errors.New("could not read socket response")
	}
	return buffer[:n], nil
}

func socketRoots(input string, existing *x509.CertPool) (*x509.CertPool, error) {
	if len(input) > 1<<20 {
		return nil, errors.New("CA PEM exceeds 1 MiB")
	}
	var roots *x509.CertPool
	if existing != nil {
		roots = existing.Clone()
	} else {
		roots, _ = x509.SystemCertPool()
	}
	if roots == nil {
		roots = x509.NewCertPool()
	}
	remaining := []byte(strings.TrimSpace(input))
	count := 0
	for len(remaining) != 0 {
		if !bytes.HasPrefix(remaining, []byte("-----BEGIN CERTIFICATE-----")) {
			return nil, errors.New("CA PEM must contain certificates only")
		}
		block, rest := pem.Decode(remaining)
		if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			return nil, errors.New("invalid CA PEM certificate")
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, errors.New("invalid CA certificate")
		}
		roots.AddCert(certificate)
		count++
		remaining = bytes.TrimSpace(rest)
	}
	if count == 0 {
		return nil, errors.New("CA PEM contains no certificates")
	}
	return roots, nil
}
