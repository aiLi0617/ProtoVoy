package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"strings"
	"time"

	"protovoy.dev/protovoy/internal/agenthistory"
	"protovoy.dev/protovoy/internal/execution"
)

type agentEvent map[string]any

type agentJob struct {
	events        chan agentEvent
	done          chan struct{}
	cancel        context.CancelFunc
	attached      bool       // protected by agentService.mu
	terminal      agentEvent // available after events is closed
	record        agenthistory.Record
	responseLimit int
	responseSeen  bool
	responseText  bool
	responseBody  []byte
}

func newExecutionID() string {
	var uuid [16]byte
	rand.Read(uuid[:])
	uuid[6] = uuid[6]&0x0f | 0x40
	uuid[8] = uuid[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", uuid[:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:])
}

func (agent *agentService) startExecution(w http.ResponseWriter, r *http.Request) {
	var envelope requestEnvelope
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			http.Error(w, "request exceeds 2 MiB", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "invalid request envelope", http.StatusBadRequest)
		}
		return
	}
	if decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "invalid request envelope", http.StatusBadRequest)
		return
	}
	plan, err := agent.prepare(envelope)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id := newExecutionID()
	agent.mu.Lock()
	if len(agent.jobs) >= 16 {
		agent.mu.Unlock()
		http.Error(w, "too many executions", http.StatusTooManyRequests)
		return
	}
	var ctx context.Context
	var cancel context.CancelFunc
	if plan.timeout == 0 {
		ctx, cancel = context.WithCancel(context.Background())
	} else {
		ctx, cancel = context.WithTimeout(context.Background(), plan.timeout)
	}
	job := &agentJob{
		events: make(chan agentEvent, 8), done: make(chan struct{}), cancel: cancel,
		record:        agenthistory.Record{ID: id, RequestID: envelope.ID, SnapshotID: envelope.historySnapshotID(), ProtocolID: envelope.Protocol},
		responseLimit: agent.history.responseLimit,
	}
	agent.jobs[id] = job
	agent.mu.Unlock()
	time.AfterFunc(45*time.Second, func() {
		agent.mu.Lock()
		defer agent.mu.Unlock()
		if agent.jobs[id] == job && !job.attached {
			delete(agent.jobs, id)
			job.cancel()
		}
	})
	go job.run(ctx, plan, agent.history)
	writeJSON(w, http.StatusAccepted, map[string]string{
		"executionId": id, "state": "running", "protocolApiVersion": apiVersion, "agentVersion": agentVersion,
	})
}

func (job *agentJob) run(ctx context.Context, plan executionPlan, history *historyStore) {
	defer close(job.events)
	defer close(job.done)
	defer job.cancel()
	started := time.Now()
	job.terminal = plan.execute(job, ctx, job.record.ID, job.record.RequestID)
	finished := time.Now()
	job.record.StartedAt, job.record.FinishedAt = started.UTC(), finished.UTC()
	job.record.DurationMS = finished.Sub(started).Milliseconds()
	job.record.State, _ = job.terminal["type"].(string)
	if job.responseLimit != 0 && job.responseSeen && (job.record.State == "completed" || len(job.responseBody) != 0) {
		size := len(job.responseBody)
		job.record.ResponseBodyBytes = &size
		job.record.ResponseBodyText = job.responseText
		job.record.ResponseBodyTruncated = int64(size) < job.record.BytesReceived
	}
	if err := history.save(job.record, job.responseBody); err != nil {
		log.Print("could not save Local Agent execution history")
	}
}

func (job *agentJob) emit(ctx context.Context, event agentEvent) bool {
	switch event["type"] {
	case "response_meta":
		if status, ok := event["status"].(int); ok {
			job.record.Status = &status
		}
		if job.responseLimit != 0 {
			value, _ := event["contentType"].(string)
			media, _, _ := mime.ParseMediaType(value)
			text := strings.HasPrefix(media, "text/") || media == "application/json" || media == "application/xml" ||
				strings.HasSuffix(media, "+json") || strings.HasSuffix(media, "+xml")
			job.responseText = (!job.responseSeen || job.responseText) && text
			job.responseSeen = true
		}
	case "response_chunk":
		if size, ok := event["size"].(int); ok {
			job.record.BytesReceived += int64(size)
		}
		if remaining := job.responseLimit - len(job.responseBody); remaining > 0 {
			if encoded, ok := event["data_base64"].(string); ok {
				if bytes, err := base64.StdEncoding.DecodeString(encoded); err == nil {
					job.responseBody = append(job.responseBody, bytes[:min(remaining, len(bytes))]...)
				}
			}
		}
	}
	select {
	case job.events <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

func (job *agentJob) emitBinary(ctx context.Context, contentType string, data []byte) bool {
	for offset := 0; offset < len(data); offset += execution.PreviewLimit {
		chunk := data[offset:min(offset+execution.PreviewLimit, len(data))]
		if !job.emit(ctx, agentEvent{
			"type": "response_chunk", "content_type": contentType, "size": len(chunk),
			"data_base64": base64.StdEncoding.EncodeToString(chunk), "done": false,
		}) {
			return false
		}
	}
	return true
}

func (job *agentJob) execute(ctx context.Context, id, requestID, protocol string, request execution.HTTPRequest) agentEvent {
	return job.executeHTTP(ctx, id, requestID, protocol, request, nil)
}

func (job *agentJob) executeHTTP(ctx context.Context, id, requestID, protocol string, request execution.HTTPRequest,
	transform func(*execution.HTTPResponse, time.Time) (*execution.HTTPResponse, error),
) agentEvent {
	started := time.Now()
	if !job.emit(ctx, agentEvent{"type": "state_changed", "state": "running", "phase": "connect"}) {
		return terminalFailure(ctx, "connection")
	}
	response, err := (execution.HTTPClient{}).Execute(ctx, request)
	if err != nil {
		return terminalFailure(ctx, "connection")
	}
	if transform != nil {
		response, err = transform(response, started)
		if err != nil {
			return terminalFailure(ctx, "post-response script")
		}
	}
	defer response.Body.Close()
	contentType := response.Headers.Get("Content-Type")
	if !job.emitResponseMeta(ctx, response) {
		return terminalFailure(ctx, "transfer")
	}
	var preview []byte
	var total int64
	buffer := make([]byte, execution.PreviewLimit)
	for {
		n, err := response.Body.Read(buffer)
		if n > 0 {
			data := buffer[:n]
			total += int64(n)
			preview = append(preview, data[:min(len(data), execution.PreviewLimit-len(preview))]...)
			if !job.emit(ctx, agentEvent{
				"type": "response_chunk", "content_type": contentType, "size": n,
				"data_base64": base64.StdEncoding.EncodeToString(data), "done": false,
			}) {
				return terminalFailure(ctx, "transfer")
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return terminalFailure(ctx, "transfer")
		}
	}
	if ctx.Err() != nil || !job.emit(ctx, agentEvent{
		"type": "response_chunk", "content_type": contentType, "size": 0,
		"preview": strings.ToValidUTF8(string(preview), "\uFFFD"), "done": true,
	}) {
		return terminalFailure(ctx, "transfer")
	}
	return completedEvent(id, requestID, protocol, response.StatusCode, total, started)
}

func (job *agentJob) emitResponseMeta(ctx context.Context, response *execution.HTTPResponse) bool {
	headers := make([][2]string, 0, len(response.Headers))
	for name, values := range response.Headers {
		for _, value := range values {
			headers = append(headers, [2]string{name, value})
		}
	}
	return job.emit(ctx, agentEvent{
		"type": "response_meta", "status": response.StatusCode, "statusText": http.StatusText(response.StatusCode),
		"contentType": response.Headers.Get("Content-Type"), "headers": headers,
	})
}

func completedEvent(id, requestID, protocol string, status int, total int64, started time.Time) agentEvent {
	finished := time.Now()
	var responseStatus any
	if status != 0 {
		responseStatus = status
	}
	return agentEvent{"type": "completed", "summary": map[string]any{
		"executionId": id, "requestId": requestID, "protocolId": protocol, "state": "completed",
		"startedAt": started.UTC(), "finishedAt": finished.UTC(), "durationMs": finished.Sub(started).Milliseconds(),
		"bytesReceived": total, "status": responseStatus,
	}}
}

func terminalFailure(ctx context.Context, phase string) agentEvent {
	switch ctx.Err() {
	case context.Canceled:
		return agentEvent{"type": "cancelled", "reason": "cancelled"}
	case context.DeadlineExceeded:
		return agentEvent{"type": "failed", "code": "timeout", "message": "request timed out"}
	default:
		return agentEvent{"type": "failed", "code": phase, "message": "request execution failed"}
	}
}

func (agent *agentService) executionEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "use GET for execution events", http.StatusMethodNotAllowed)
		return
	}
	id := r.PathValue("id")
	agent.mu.Lock()
	job := agent.jobs[id]
	if job == nil || job.attached {
		agent.mu.Unlock()
		http.Error(w, "execution not found or events already claimed", http.StatusNotFound)
		return
	}
	job.attached = true
	agent.mu.Unlock()
	defer func() {
		job.cancel()
		agent.mu.Lock()
		delete(agent.jobs, id)
		agent.mu.Unlock()
	}()
	w.Header().Set("Content-Type", "text/event-stream")
	controller := http.NewResponseController(w)
	write := func(frame string) error {
		if err := controller.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return err
		}
		if _, err := io.WriteString(w, frame); err != nil {
			return err
		}
		return controller.Flush()
	}
	send := func(event agentEvent) error {
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}
		return write("event: execution\ndata: " + string(data) + "\n\n")
	}
	if err := write(": connected\n\n"); err != nil {
		return
	}
	keepalive := time.NewTicker(5 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			if write(": keepalive\n\n") != nil {
				return
			}
		case event, ok := <-job.events:
			if !ok {
				_ = send(job.terminal)
				return
			}
			if send(event) != nil {
				return
			}
		}
	}
}

func (agent *agentService) cancelExecution(w http.ResponseWriter, r *http.Request) {
	agent.mu.Lock()
	job := agent.jobs[r.PathValue("id")]
	agent.mu.Unlock()
	cancelled := false
	if job != nil {
		select {
		case <-job.done:
		default:
			job.cancel()
			cancelled = true
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"cancelled": cancelled})
}
