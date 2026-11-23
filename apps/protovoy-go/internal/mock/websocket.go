//go:build !js

package mock

import (
	"context"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

func (app *service) serveWebSocket(w http.ResponseWriter, r *http.Request, rule *Rule) {
	client, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer client.CloseNow()
	app.active.Add(1)
	defer app.active.Add(-1)
	client.SetReadLimit(maxBodyBytes)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	wait := func(milliseconds uint64) bool {
		timer := time.NewTimer(time.Duration(milliseconds) * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
			return true
		}
	}
	if !wait(rule.DelayMS) {
		return
	}
	for i, message := range rule.WSMessages {
		if i > 0 && !wait(rule.WSIntervalMS) {
			return
		}
		if client.Write(ctx, websocket.MessageText, []byte(message)) != nil {
			return
		}
	}
	if rule.WSEcho {
		for {
			kind, message, err := client.Read(ctx)
			if err != nil || client.Write(ctx, kind, message) != nil {
				return
			}
		}
	}
	_ = client.Close(websocket.StatusNormalClosure, "mock messages completed")
}
