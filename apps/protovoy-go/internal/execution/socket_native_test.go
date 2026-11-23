//go:build !js

package execution

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTCPCancellationClosesPendingRead(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	closed := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			closed <- err
			return
		}
		defer connection.Close()
		_ = connection.SetDeadline(time.Now().Add(time.Second))
		one := make([]byte, 1)
		_, _ = io.ReadFull(connection, one)
		cancel()
		_, err = connection.Read(one)
		closed <- err
	}()
	_, err = ExchangeSocket(ctx, SocketRequest{Network: "tcp", Target: listener.Addr().String(), Data: "x", Framing: "fixed", FixedLength: 5})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation returned %v", err)
	}
	if err := <-closed; err != io.EOF {
		t.Fatalf("peer did not observe EOF: %v", err)
	}
}

func TestTCPRepeatedSendsReuseConnection(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		_ = connection.SetDeadline(time.Now().Add(time.Second))
		buffer := make([]byte, 6)
		if _, err := io.ReadFull(connection, buffer); err == nil {
			_, _ = connection.Write(buffer)
		}
	}()
	data, err := ExchangeSocket(context.Background(), SocketRequest{Network: "tcp", Target: listener.Addr().String(), Data: "ab", SendCount: 3, Interval: time.Millisecond, Framing: "fixed", FixedLength: 6, Timeout: time.Second})
	if err != nil || string(data) != "ababab" {
		t.Fatalf("repeated sends: %q, %v", data, err)
	}
}

func TestTCPCancelDuringRepeatInterval(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	closed := make(chan bool, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			closed <- false
			return
		}
		defer connection.Close()
		_ = connection.SetDeadline(time.Now().Add(time.Second))
		buffer := make([]byte, 1)
		_, err = io.ReadFull(connection, buffer)
		if err != nil {
			closed <- false
			return
		}
		cancel()
		n, err := connection.Read(buffer)
		closed <- n == 0 && err == io.EOF
	}()
	_, err = ExchangeSocket(ctx, SocketRequest{Network: "tcp", Target: listener.Addr().String(), Data: "x", SendCount: 2, Interval: time.Minute})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("interval cancellation: %v", err)
	}
	if !<-closed {
		t.Fatal("a second payload was sent or the connection stayed open")
	}
}

func TestUDPEmptyDatagram(t *testing.T) {
	listener, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		buffer := make([]byte, 1)
		_, peer, err := listener.ReadFrom(buffer)
		if err == nil {
			_, _ = listener.WriteTo([]byte("ok"), peer)
		}
	}()
	data, err := ExchangeSocket(context.Background(), SocketRequest{Network: "udp", Target: listener.LocalAddr().String(), Timeout: time.Second})
	if err != nil || string(data) != "ok" {
		t.Fatalf("empty datagram response: %q, %v", data, err)
	}
}

func TestTCPTLSVerifiesPeer(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "verified") }))
	defer server.Close()
	request := SocketRequest{Network: "tcp", Target: strings.TrimPrefix(server.URL, "https://"), TLS: true, Data: "GET / HTTP/1.0\r\n\r\n", Timeout: time.Second}
	if _, err := ExchangeSocket(context.Background(), request); err == nil {
		t.Fatal("untrusted certificate accepted")
	}
	request.RootCAs = x509.NewCertPool()
	request.RootCAs.AddCert(server.Certificate())
	data, err := ExchangeSocket(context.Background(), request)
	if err != nil || !strings.Contains(string(data), "200 OK") {
		t.Fatalf("trusted TLS exchange: %q, %v", data, err)
	}
	request.RootCAs = nil
	request.CAPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))
	if _, err := ExchangeSocket(context.Background(), request); err != nil {
		t.Fatalf("PEM CA was not used: %v", err)
	}
	request.CAPEM += "\n-----BEGIN PRIVATE KEY-----\ninvalid\n-----END PRIVATE KEY-----"
	if _, err := ExchangeSocket(context.Background(), request); err == nil {
		t.Fatal("private key accepted as CA input")
	}
	request.CAPEM = ""
	request.RootCAs = x509.NewCertPool()
	request.RootCAs.AddCert(server.Certificate())
	request.ServerName = "wrong.invalid"
	if _, err := ExchangeSocket(context.Background(), request); err == nil {
		t.Fatal("wrong certificate name accepted")
	}
}

func TestTCPFramingAcrossWrites(t *testing.T) {
	for _, framing := range []string{"fixed", "delimiter"} {
		t.Run(framing, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			go func() {
				connection, err := listener.Accept()
				if err != nil {
					return
				}
				defer connection.Close()
				request := make([]byte, 1)
				if _, err := io.ReadFull(connection, request); err != nil {
					return
				}
				_, _ = connection.Write([]byte("abc\r"))
				time.Sleep(20 * time.Millisecond)
				_, _ = connection.Write([]byte("\nextra"))
			}()
			data, err := ExchangeSocket(context.Background(), SocketRequest{Network: "tcp", Target: listener.Addr().String(), Data: "x", Framing: framing, Delimiter: "\r\n", FixedLength: 5, Timeout: time.Second})
			if err != nil || string(data) != "abc\r\n" {
				t.Fatalf("framed response = %q, %v", data, err)
			}
		})
	}
}

func TestUDPExchangeDecodesBinary(t *testing.T) {
	listener, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		buffer := make([]byte, 100)
		n, peer, err := listener.ReadFrom(buffer)
		if err == nil {
			_, _ = listener.WriteTo(buffer[:n], peer)
		}
	}()
	data, err := ExchangeSocket(context.Background(), SocketRequest{Network: "udp", Target: listener.LocalAddr().String(), Data: "00 01 ff", Encoding: "hex", Timeout: time.Second})
	if err != nil || string(data) != string([]byte{0, 1, 255}) {
		t.Fatalf("UDP echo: %v, %v", data, err)
	}
}
