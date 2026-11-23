//go:build !js

package execution

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestAMQPPublishAndConsume(t *testing.T) {
	for _, scenario := range []string{"publish", "publish_returned", "consume", "consume_cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			_ = listener.SetDeadline(time.Now().Add(5 * time.Second))
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			peer := make(chan error, 1)
			go func() { peer <- amqpFixture(listener, scenario, cancel) }()
			mode, _, _ := strings.Cut(scenario, "_")
			options := AMQPOptions{Mode: mode, ExchangeType: "direct", RoutingKey: "jobs", Queue: "jobs",
				Declare: true, ReceiveLimit: 1, Encoding: "base64", Payload: "AP8=", ContentType: "application/octet-stream"}
			request, err := PrepareAMQP("amqp://"+listener.Addr().String()+"/%2F", options)
			if err != nil {
				t.Fatal(err)
			}
			var result map[string]any
			err = request.Execute(ctx, func() error { return nil }, func(value any) error {
				result = value.(map[string]any)
				return nil
			})
			if peerErr := <-peer; peerErr != nil {
				t.Fatal(peerErr)
			}
			if scenario == "consume_cancelled" {
				if !errors.Is(err, context.Canceled) || result != nil {
					t.Fatalf("AMQP consume ignored cancellation: %+v, %v", result, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "publish":
				if result["published"] != true || result["confirmation"] != "ack" {
					t.Fatalf("confirmed message was not reported as published: %+v", result)
				}
			case "publish_returned":
				if result["published"] != false || result["confirmation"] != "returned" || result["replyCode"] != uint16(312) {
					t.Fatalf("unroutable message reported as published: %+v", result)
				}
			case "consume":
				if result["payload"] != nil || result["payloadBase64"] != "AP8=" || result["routingKey"] != "jobs" {
					t.Fatalf("binary delivery was lost: %+v", result)
				}
			}
		})
	}
}

func amqpFixture(listener net.Listener, scenario string, cancel context.CancelFunc) error {
	publish := strings.HasPrefix(scenario, "publish")
	conn, err := listener.Accept()
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	protocol := make([]byte, 8)
	if _, err := io.ReadFull(conn, protocol); err != nil || string(protocol) != "AMQP\x00\x00\x09\x01" {
		return fmt.Errorf("invalid AMQP protocol header: %x (%v)", protocol, err)
	}
	send := func(kind byte, channel uint16, data []byte) error {
		frame := binary.BigEndian.AppendUint16([]byte{kind}, channel)
		frame = binary.BigEndian.AppendUint32(frame, uint32(len(data)))
		frame = append(append(frame, data...), 0xce)
		_, err := conn.Write(frame)
		return err
	}
	receive := func(kind byte, channel uint16) ([]byte, error) {
		var header [7]byte
		if _, err := io.ReadFull(conn, header[:]); err != nil {
			return nil, err
		}
		length := binary.BigEndian.Uint32(header[3:])
		if header[0] != kind || binary.BigEndian.Uint16(header[1:]) != channel || length > amqpFrameLimit {
			return nil, fmt.Errorf("unexpected AMQP frame header: %x", header)
		}
		data := make([]byte, int(length)+1)
		if _, err := io.ReadFull(conn, data); err != nil {
			return nil, err
		}
		return data[:length], nil
	}
	method := func(channel, class, code uint16, args string) error {
		data := binary.BigEndian.AppendUint16(nil, class)
		return send(1, channel, append(binary.BigEndian.AppendUint16(data, code), args...))
	}
	expect := func(channel, class, code uint16) ([]byte, error) {
		data, err := receive(1, channel)
		want := binary.BigEndian.AppendUint16(binary.BigEndian.AppendUint16(nil, class), code)
		if err != nil || !bytes.HasPrefix(data, want) {
			return nil, fmt.Errorf("expected AMQP method %d/%d, got %x (%v)", class, code, data, err)
		}
		return data[4:], nil
	}
	if err := method(0, 10, 10, "\x00\x09\x00\x00\x00\x00\x00\x00\x00\x05PLAIN\x00\x00\x00\x05en_US"); err != nil {
		return err
	}
	for _, step := range []struct {
		channel, class, request, response uint16
		args                              string
	}{
		{0, 10, 11, 30, "\x00\x01\x00\x02\x00\x00\x00\x00"},
		{0, 10, 31, 0, ""},
		{0, 10, 40, 41, "\x00"},
		{1, 20, 10, 11, "\x00\x00\x00\x00"},
	} {
		if _, err := expect(step.channel, step.class, step.request); err != nil {
			return err
		}
		if step.response != 0 {
			if err := method(step.channel, step.class, step.response, step.args); err != nil {
				return err
			}
		}
	}
	const tag = "\x00\x00\x00\x00\x00\x00\x00\x01"
	if publish {
		if _, err := expect(1, 85, 10); err != nil {
			return err
		}
		if err := method(1, 85, 11, ""); err != nil {
			return err
		}
		published, err := expect(1, 60, 40)
		if err != nil || string(published) != "\x00\x00\x00\x04jobs\x01" {
			return fmt.Errorf("publish must set mandatory: %x (%v)", published, err)
		}
		if _, err := receive(2, 1); err != nil {
			return err
		}
		if data, err := receive(3, 1); err != nil || !bytes.Equal(data, []byte{0, 255}) {
			return fmt.Errorf("unexpected AMQP publish payload: %x (%v)", data, err)
		}
		if scenario == "publish_returned" {
			if err := method(1, 60, 50, "\x018\x08NO_ROUTE\x00\x04jobs"); err != nil {
				return err
			}
		}
	} else {
		if _, err := expect(1, 50, 10); err != nil {
			return err
		}
		if err := method(1, 50, 11, "\x04jobs\x00\x00\x00\x00\x00\x00\x00\x00"); err != nil {
			return err
		}
		if _, err := expect(1, 60, 10); err != nil {
			return err
		}
		if err := method(1, 60, 11, ""); err != nil {
			return err
		}
		input, err := expect(1, 60, 20)
		if err != nil || len(input) < 8 || len(input) < 8+int(input[7]) {
			return fmt.Errorf("invalid AMQP consumer request: %x (%v)", input, err)
		}
		consumer := string(input[7 : 8+int(input[7])])
		if err := method(1, 60, 21, consumer); err != nil {
			return err
		}
		if scenario == "consume_cancelled" {
			cancel()
			n, err := conn.Read(make([]byte, 1))
			// Winsock reports WSAECONNRESET when the peer closes with unread data.
			const winsockConnectionReset = syscall.Errno(10054)
			reset := errors.Is(err, syscall.ECONNRESET) ||
				(runtime.GOOS == "windows" && errors.Is(err, winsockConnectionReset))
			if n != 0 || (!errors.Is(err, io.EOF) && !reset) {
				return fmt.Errorf("cancellation did not close the AMQP connection: %v", err)
			}
			return nil
		}
		if err := method(1, 60, 60, consumer+tag+"\x00\x00\x04jobs"); err != nil {
			return err
		}
	}
	if scenario != "publish" {
		if err := send(2, 1, []byte("\x00\x3c\x00\x00\x00\x00\x00\x00\x00\x00\x00\x02\x00\x00")); err != nil {
			return err
		}
		if err := send(3, 1, []byte{0, 255}); err != nil {
			return err
		}
	}
	if publish {
		if err := method(1, 60, 80, tag+"\x00"); err != nil {
			return err
		}
	} else if ack, err := expect(1, 60, 80); err != nil || string(ack) != tag+"\x00" {
		return fmt.Errorf("unexpected delivery acknowledgement: %x (%v)", ack, err)
	}
	if _, err := expect(0, 10, 50); err != nil {
		return err
	}
	return method(0, 10, 51, "")
}

func TestAMQPRejectsUnboundedBrokerLengths(t *testing.T) {
	// connection.start containing a field table with an impossible nested string.
	start := []byte("\x00\x0a\x00\x0a\x00\x09\x00\x00\x00\x07\x01xS\x7f\xff\xff\xff\x00\x00\x00\x05PLAIN\x00\x00\x00\x05en_US")
	ack := []byte("\x00\x3c\x00\x50\xff\xff\xff\xff\xff\xff\xff\xff\x01")
	header := []byte("\x00\x3c\x00\x00\x00\x00\x00\x00\x00\x10\x00\x01\x00\x00")
	conn := amqpBoundedConn{}
	if validAMQPMethod(start) || validAMQPMethod(ack) || conn.validFrame(2, header) {
		t.Fatal("accepted an AMQP frame with unbounded broker-controlled work")
	}
}
