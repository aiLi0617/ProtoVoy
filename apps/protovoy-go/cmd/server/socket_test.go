package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"protovoy.dev/protovoy/internal/execution"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSocketAPITimeoutIncludesSendInterval(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	received := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		data, _ := io.ReadAll(conn)
		received <- string(data)
	}()
	const addr = "127.0.0.1:39300"
	api := handler(loopbackAccess(addr), t.TempDir(), "", "", socketTargets{TCP: listener.Addr().String()})
	r := httptest.NewRequest(http.MethodPost, "http://"+addr+"/api/socket",
		strings.NewReader(`{"network":"tcp","data":"ping","sendCount":2,"intervalMs":500,"timeoutMs":100}`))
	r.Header.Set("Origin", "http://"+addr)
	r.Header.Set("X-ProtoVoy-Preview", "1")
	w := httptest.NewRecorder()
	api.ServeHTTP(w, r)
	if w.Code != http.StatusGatewayTimeout {
		t.Fatalf("got HTTP %d, want timeout", w.Code)
	}
	select {
	case data := <-received:
		if data != "ping" {
			t.Fatalf("unexpected sends before timeout: %q", data)
		}
	case <-time.After(time.Second):
		t.Fatal("upstream did not observe the exchange")
	}
}

func TestSocketAPITLSPrivateCAAndNameValidation(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "tls-ok") }))
	defer upstream.Close()
	certificate := upstream.Certificate()
	ca := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw}))
	const addr = "127.0.0.1:39300"
	target := strings.TrimPrefix(upstream.URL, "https://")
	api := handler(loopbackAccess(addr), t.TempDir(), "", "", socketTargets{TCP: target})
	for _, tc := range []struct {
		name, ca, serverName string
		status               int
	}{
		{"untrusted", "", "", http.StatusBadGateway},
		{"trusted", ca, "", http.StatusOK},
		{"wrong name", ca, "wrong.invalid", http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(execution.SocketPayload{Network: "tcp", TLS: true, CAPEM: tc.ca, ServerName: tc.serverName, Data: "GET / HTTP/1.0\r\n\r\n"})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "http://"+addr+"/api/socket", bytes.NewReader(body))
			request.Header.Set("Origin", "http://"+addr)
			request.Header.Set("X-ProtoVoy-Preview", "1")
			response := httptest.NewRecorder()
			api.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status %d, want %d", response.Code, tc.status)
			}
			if tc.status == http.StatusOK {
				var result execution.SocketResponse
				if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				data, err := base64.StdEncoding.DecodeString(result.Data)
				if err != nil || !bytes.Contains(data, []byte("200 OK")) {
					t.Fatal("TLS response was not returned")
				}
			}
		})
	}
}

func TestUDPAPIUsesDatagramBoundary(t *testing.T) {
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
	const addr = "127.0.0.1:39300"
	api := handler(loopbackAccess(addr), t.TempDir(), "", "", socketTargets{UDP: listener.LocalAddr().String()})
	request := httptest.NewRequest(http.MethodPost, "http://"+addr+"/api/socket", strings.NewReader(`{"network":"udp","encoding":"base64","data":"AAH/"}`))
	request.Header.Set("Origin", "http://"+addr)
	request.Header.Set("X-ProtoVoy-Preview", "1")
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	var body struct {
		Data string `json:"dataBase64"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil || body.Data != "AAH/" {
		t.Fatalf("UDP API response: %d, %s", response.Code, response.Body.String())
	}
}

func TestSocketAPIValidatesBeforeDialAndReturnsBinary(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var connections atomic.Int32
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		connections.Add(1)
		defer connection.Close()
		_ = connection.SetDeadline(time.Now().Add(time.Second))
		buffer := make([]byte, 6)
		if _, err := io.ReadFull(connection, buffer); err == nil {
			_, _ = connection.Write(buffer)
		}
	}()
	const addr = "127.0.0.1:39300"
	api := handler(loopbackAccess(addr), t.TempDir(), "", "", socketTargets{TCP: listener.Addr().String()})
	for _, test := range []struct {
		name, origin, marker, body string
		status                     int
	}{
		{"foreign origin", "https://other.test", "1", `{}`, http.StatusForbidden},
		{"missing marker", "http://" + addr, "", `{}`, http.StatusForbidden},
		{"mismatch", "http://" + addr, "1", `{"network":"tcp","expectedTarget":"other:90"}`, http.StatusConflict},
		{"unconfigured UDP", "http://" + addr, "1", `{"network":"udp"}`, http.StatusServiceUnavailable},
		{"unknown target", "http://" + addr, "1", `{"network":"tcp","target":"other:90"}`, http.StatusBadRequest},
		{"multiple documents", "http://" + addr, "1", `{"network":"tcp"} {}`, http.StatusBadRequest},
		{"repeated echo", "http://" + addr, "1", `{"network":"tcp","data":"0001ff","encoding":"hex","framing":"fixed","fixedLength":6,"sendCount":2,"intervalMs":1}`, http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "http://"+addr+"/api/socket", strings.NewReader(test.body))
			request.Header.Set("Origin", test.origin)
			request.Header.Set("X-ProtoVoy-Preview", test.marker)
			response := httptest.NewRecorder()
			api.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status %d, want %d", response.Code, test.status)
			}
			if test.status != http.StatusOK {
				if connections.Load() != 0 {
					t.Fatal("rejected request reached the TCP target")
				}
				return
			}
			var body struct {
				Data string `json:"dataBase64"`
			}
			if json.Unmarshal(response.Body.Bytes(), &body) != nil || body.Data != "AAH/AAH/" || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("binary response was not preserved")
			}
		})
	}
}
