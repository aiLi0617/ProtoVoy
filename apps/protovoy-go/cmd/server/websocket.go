package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"protovoy.dev/protovoy/internal/execution"
)

type webSocketBridge struct {
	mu       sync.Mutex
	active   sync.WaitGroup
	stopping chan struct{}
}

func (bridge *webSocketBridge) admit() bool {
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	select {
	case <-bridge.stopping:
		return false
	default:
		bridge.active.Add(1)
		return true
	}
}

func (bridge *webSocketBridge) stop() <-chan struct{} {
	bridge.mu.Lock()
	select {
	case <-bridge.stopping:
	default:
		close(bridge.stopping)
	}
	bridge.mu.Unlock()
	drained := make(chan struct{})
	go func() {
		bridge.active.Wait()
		close(drained)
	}()
	return drained
}

func (bridge *webSocketBridge) handler(origin, target string) http.HandlerFunc {
	slots := make(chan struct{}, 16)
	return func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r, origin) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if target == "" {
			http.Error(w, "WebSocket endpoint is not configured", http.StatusServiceUnavailable)
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			http.Error(w, "too many active WebSocket connections", http.StatusServiceUnavailable)
			return
		}
		if !bridge.admit() {
			http.Error(w, "WebSocket bridge is shutting down", http.StatusServiceUnavailable)
			return
		}
		defer bridge.active.Done()
		client, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		finished, closed := make(chan struct{}), make(chan struct{})
		dialContext, cancelDial := context.WithCancel(context.Background())
		defer cancelDial()
		go func() {
			defer close(closed)
			select {
			case <-bridge.stopping:
				cancelDial()
				_ = client.Close(websocket.StatusGoingAway, "server shutting down")
			case <-finished:
				_ = client.CloseNow()
			}
		}()
		defer func() {
			close(finished)
			<-closed
		}()

		setupCtx, cancelSetup := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancelSetup()
		client.SetReadLimit(execution.WebSocketSetupLimit)
		kind, data, err := client.Read(setupCtx)
		if err != nil {
			return
		}
		var setup execution.WebSocketSetup
		if kind != websocket.MessageText || json.Unmarshal(data, &setup) != nil {
			_ = wsjson.Write(setupCtx, client, execution.WebSocketReady{Error: "invalid WebSocket setup"})
			return
		}
		if setup.ExpectedTarget != "" && setup.ExpectedTarget != target {
			_ = wsjson.Write(setupCtx, client, execution.WebSocketReady{Error: "WebSocket URL differs from the server-configured endpoint"})
			return
		}
		cancelSetup()
		execution.RelayWebSocket(dialContext, client, target, setup)
	}
}
