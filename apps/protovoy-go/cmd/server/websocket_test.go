package main

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"protovoy.dev/protovoy/internal/execution"
)

func TestWebSocketBridgeStopIsRepeatable(t *testing.T) {
	bridge := &webSocketBridge{stopping: make(chan struct{})}
	if !bridge.admit() {
		t.Fatal("bridge rejected initial connection")
	}
	first := bridge.stop()
	second := bridge.stop()
	if bridge.admit() {
		t.Fatal("bridge admitted a connection after stopping")
	}
	select {
	case <-first:
		t.Fatal("bridge drained before active connection ended")
	default:
	}
	bridge.active.Done()
	for _, drained := range []<-chan struct{}{first, second} {
		select {
		case <-drained:
		case <-time.After(time.Second):
			t.Fatal("repeated stop did not report drain completion")
		}
	}
}

func TestWebSocketBridgeShutdownCancelsPendingUpstreamHandshake(t *testing.T) {
	started, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
			close(cancelled)
		case <-release:
		}
	}))
	defer upstream.Close()
	defer close(release)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	addr := listener.Addr().String()
	origin := "http://" + addr
	target := "ws" + strings.TrimPrefix(upstream.URL, "http")
	application := handler(loopbackAccess(addr), t.TempDir(), "", target)
	server := &http.Server{Handler: application}
	defer server.Close()
	runContext, stop := context.WithCancel(context.Background())
	defer stop()
	exited := make(chan error, 1)
	go func() {
		exited <- serveUntilShutdown(runContext, server, func() error { return server.Serve(listener) }, application.bridge.stop)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, _, err := websocket.Dial(ctx, "ws://"+addr+execution.WebSocketBridgePath,
		&websocket.DialOptions{HTTPHeader: http.Header{"Origin": {origin}}})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseNow()
	if err := wsjson.Write(ctx, client, execution.WebSocketSetup{ExpectedTarget: target}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("upstream handshake did not start")
	}
	stop()
	if _, _, err := client.Read(ctx); websocket.CloseStatus(err) != websocket.StatusGoingAway {
		t.Fatalf("expected a Going Away frame during the upstream handshake, got %v", err)
	} else {
		var closed websocket.CloseError
		if !errors.As(err, &closed) || closed.Reason != "server shutting down" {
			t.Fatalf("missing shutdown reason during handshake: %v", err)
		}
	}
	select {
	case <-cancelled:
	case <-ctx.Done():
		t.Fatal("upstream handshake was not cancelled")
	}
	select {
	case err := <-exited:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("server did not exit")
	}
}

func TestWebSocketBridgeEchoAndFixedTarget(t *testing.T) {
	var dialCount atomic.Int32
	disconnected := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dialCount.Add(1)
		if r.Header.Get("X-Trace") != "fixture" {
			t.Error("upstream handshake lost a configured header")
		}
		connection, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"fixture"}})
		if err != nil {
			return
		}
		defer connection.CloseNow()
		defer close(disconnected)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for {
			kind, data, err := connection.Read(ctx)
			if err != nil || connection.Write(ctx, kind, data) != nil {
				return
			}
		}
	}))
	defer upstream.Close()
	target := "ws" + strings.TrimPrefix(upstream.URL, "http")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	application := handler(loopbackAccess(addr), t.TempDir(), "", target)
	bridge := httptest.NewUnstartedServer(application)
	bridge.Listener.Close()
	bridge.Listener = listener
	bridge.Start()
	defer bridge.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dial := func(origin string) (*websocket.Conn, *http.Response, error) {
		return websocket.Dial(ctx, "ws://"+addr+execution.WebSocketBridgePath, &websocket.DialOptions{
			HTTPHeader: http.Header{"Origin": []string{origin}},
		})
	}
	connection, rejected, err := dial("https://example.test")
	if connection != nil || err == nil || rejected == nil || rejected.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin upgrade was not forbidden: %v", err)
	}
	connection, _, err = dial("http://" + addr)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.CloseNow()
	var ready execution.WebSocketReady
	if err := wsjson.Write(ctx, connection, execution.WebSocketSetup{ExpectedTarget: target + "/other"}); err != nil {
		t.Fatal(err)
	}
	if err := wsjson.Read(ctx, connection, &ready); err != nil || ready.Error == "" || dialCount.Load() != 0 {
		t.Fatalf("mismatched target was not rejected before dialing: %v, %+v", err, ready)
	}
	connection.CloseNow()

	connection, _, err = dial("http://" + addr)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.CloseNow()
	if err := wsjson.Write(ctx, connection, execution.WebSocketSetup{
		ExpectedTarget: target, Headers: [][2]string{{"X-Trace", "fixture"}}, Subprotocols: []string{"fixture"},
	}); err != nil {
		t.Fatal(err)
	}
	ready = execution.WebSocketReady{}
	if err := wsjson.Read(ctx, connection, &ready); err != nil || ready.Subprotocol != "fixture" || ready.Error != "" {
		t.Fatalf("unexpected bridge handshake: %v, %+v", err, ready)
	}
	for _, frame := range []struct {
		kind websocket.MessageType
		data []byte
	}{
		{websocket.MessageText, []byte("你好")},
		{websocket.MessageBinary, []byte{0, 1, 255}},
	} {
		if err := connection.Write(ctx, frame.kind, frame.data); err != nil {
			t.Fatal(err)
		}
		kind, data, err := connection.Read(ctx)
		if err != nil || kind != frame.kind || !bytes.Equal(data, frame.data) {
			t.Fatalf("bridge changed the frame: %v, %v, %v", err, kind, data)
		}
	}
	drained := application.bridge.stop()
	if _, _, err := connection.Read(ctx); websocket.CloseStatus(err) != websocket.StatusGoingAway {
		t.Fatalf("expected a Going Away frame during shutdown, got %v", err)
	} else {
		var closed websocket.CloseError
		if !errors.As(err, &closed) || closed.Reason != "server shutting down" {
			t.Fatalf("missing shutdown reason after relay: %v", err)
		}
	}
	if late, rejected, err := dial("http://" + addr); err == nil || rejected == nil || rejected.StatusCode != http.StatusServiceUnavailable {
		if late != nil {
			late.CloseNow()
		}
		t.Fatalf("bridge accepted an upgrade during shutdown: %v", err)
	}
	select {
	case <-drained:
	case <-ctx.Done():
		t.Fatal("bridge did not drain")
	}
	select {
	case <-disconnected:
	case <-ctx.Done():
		t.Fatal("bridge shutdown did not disconnect the upstream")
	}
}
