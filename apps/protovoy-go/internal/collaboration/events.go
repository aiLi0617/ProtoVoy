//go:build !js

package collaboration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type workspaceChanged struct {
	WorkspaceID string    `json:"workspaceId"`
	Revision    int64     `json:"revision"`
	ActorID     string    `json:"actorId"`
	At          time.Time `json:"at"`
}

type commentChanged struct {
	WorkspaceID string    `json:"workspaceId"`
	CommentID   string    `json:"commentId"`
	Action      string    `json:"action"`
	ActorID     string    `json:"actorId"`
	At          time.Time `json:"at"`
}

type teamEvent struct {
	name, id string
	data     any
}

type workspaceEvents struct {
	mu          sync.Mutex
	subscribers map[chan teamEvent]string
}

func (events *workspaceEvents) subscribe(org string) (chan teamEvent, func()) {
	events.mu.Lock()
	defer events.mu.Unlock()
	if len(events.subscribers) >= 256 {
		return nil, nil
	}
	if events.subscribers == nil {
		events.subscribers = make(map[chan teamEvent]string)
	}
	stream := make(chan teamEvent, 16)
	events.subscribers[stream] = org
	return stream, func() {
		events.mu.Lock()
		delete(events.subscribers, stream)
		events.mu.Unlock()
	}
}

func (events *workspaceEvents) publish(org string, event teamEvent) {
	events.mu.Lock()
	defer events.mu.Unlock()
	for stream, organization := range events.subscribers {
		if org != organization {
			continue
		}
		select {
		case stream <- event:
		default:
			delete(events.subscribers, stream)
			close(stream)
		}
	}
}

func (store *WorkspaceStore) RegisterEventRoute(ctx context.Context, mux *http.ServeMux, access WorkspaceAccess) {
	mux.HandleFunc("GET /v1/organizations/{organizationId}/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		org := r.PathValue("organizationId")
		if _, denial := access(r, org, false); denial != 0 {
			http.Error(w, http.StatusText(denial), denial)
			return
		}
		stream, unsubscribe := store.events.subscribe(org)
		if stream == nil {
			http.Error(w, "too many team event streams", http.StatusServiceUnavailable)
			return
		}
		defer unsubscribe()
		controller := http.NewResponseController(w)
		send := func(message string) bool {
			if err := controller.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
				return false
			}
			if _, err := fmt.Fprint(w, message); err != nil {
				return false
			}
			return controller.Flush() == nil
		}
		w.Header().Set("Content-Type", "text/event-stream")
		connected := fmt.Sprintf("event: connected\ndata: {\"at\":%q}\n\n", time.Now().UTC().Format(time.RFC3339Nano))
		if !send(connected) {
			return
		}
		heartbeat := time.NewTicker(15 * time.Second)
		defer heartbeat.Stop()
		session := time.NewTimer(30 * time.Minute)
		defer session.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-r.Context().Done():
				return
			case <-session.C:
				return
			case <-heartbeat.C:
				if _, denial := access(r, org, false); denial != 0 || !send(": keepalive\n\n") {
					return
				}
			case event, ok := <-stream:
				if !ok {
					return
				}
				if _, denial := access(r, org, false); denial != 0 {
					return
				}
				data, _ := json.Marshal(event.data)
				id := ""
				if !strings.ContainsAny(event.id, "\x00\r\n") {
					id = "id: " + event.id + "\n"
				}
				if !send(id + "event: " + event.name + "\ndata: " + string(data) + "\n\n") {
					return
				}
			}
		}
	})
}
