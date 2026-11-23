package execution

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestSSEDecoderFieldsAndResumeState(t *testing.T) {
	input := "\uFEFF: heartbeat\r\nretry: 25\r\nid: 5\r\nevent: update\r\ndata: 第一行\r\ndata: second\r\n\r\n" +
		"id:\revent: ignored\r\rdata:\r\rid: incomplete\rdata: pending"
	decoder := NewSSEDecoder(strings.NewReader(input), "initial", time.Second)
	first, err := decoder.Next()
	if err != nil || first != (SSEEvent{Type: "update", ID: "5", Data: "第一行\nsecond"}) || decoder.RetryDelay != 25*time.Millisecond {
		t.Fatalf("unexpected first event: %+v, %v, retry=%v", first, err, decoder.RetryDelay)
	}
	second, err := decoder.Next()
	if err != nil || second != (SSEEvent{Type: "message"}) || decoder.LastEventID != "" {
		t.Fatalf("id reset and empty event failed: %+v, %v", second, err)
	}
	if _, err := decoder.Next(); !errors.Is(err, io.EOF) || decoder.LastEventID != "" {
		t.Fatalf("partial event should be discarded: %v, id=%q", err, decoder.LastEventID)
	}
}

func TestSSEDecoderBoundsBufferedEvent(t *testing.T) {
	for _, input := range []string{
		"data: " + strings.Repeat("a", maxSSEEventBytes+1),
		strings.Repeat("data: 0123456789\n", maxSSEEventBytes/10) + "\n",
	} {
		decoder := NewSSEDecoder(strings.NewReader(input), "", time.Second)
		if _, err := decoder.Next(); err == nil || errors.Is(err, io.EOF) {
			t.Fatalf("oversize event must fail: %v", err)
		}
	}
}
