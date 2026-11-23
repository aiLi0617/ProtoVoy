//go:build !js

package execution

import (
	"bufio"
	"context"
	"crypto/tls"
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

func TestRedisSetupPrecedesCommands(t *testing.T) {
	for _, reject := range []bool{false, true} {
		name := "accepted"
		if reject {
			name = "rejected"
		}
		t.Run(name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			peer := make(chan error, 1)
			go func() {
				connection, err := listener.Accept()
				if err != nil {
					peer <- err
					return
				}
				defer connection.Close()
				_ = connection.SetDeadline(time.Now().Add(5 * time.Second))
				read := func(want string) error {
					data := make([]byte, len(want))
					if _, err := io.ReadFull(connection, data); err != nil {
						return err
					}
					if string(data) != want {
						return errors.New("unexpected Redis command")
					}
					return nil
				}
				if err := read("*3\r\n$4\r\nAUTH\r\n$4\r\nuser\r\n$7\r\nfixture\r\n"); err != nil {
					peer <- err
					return
				}
				if reject {
					_, _ = io.WriteString(connection, "-ERR fixture\r\n")
					var next [1]byte
					if _, err := connection.Read(next[:]); err != io.EOF {
						peer <- errors.New("connection did not close immediately after rejected AUTH")
					} else {
						peer <- nil
					}
					return
				}
				_, _ = io.WriteString(connection, "+OK\r\n")
				if err := read("*2\r\n$5\r\nHELLO\r\n$1\r\n3\r\n"); err != nil {
					peer <- err
					return
				}
				_, _ = io.WriteString(connection, "%1\r\n$5\r\nproto\r\n:3\r\n")
				if err := read("*2\r\n$6\r\nSELECT\r\n$1\r\n2\r\n"); err != nil {
					peer <- err
					return
				}
				_, _ = io.WriteString(connection, "+OK\r\n")
				if err := read("*1\r\n$4\r\nPING\r\n"); err != nil {
					peer <- err
					return
				}
				_, err = io.WriteString(connection, ">2\r\n+invalidate\r\n*1\r\n+key\r\n|1\r\n+ttl\r\n:20\r\n+PONG\r\n")
				peer <- err
			}()
			options := RedisOptions{Protocol: 3, Username: "user", Password: "fixture", Database: "2"}
			values, err := ExecuteRedisWithOptions(context.Background(), listener.Addr().String(), [][]string{{"PING"}}, 5*time.Second, options)
			if reject {
				if err == nil || strings.Contains(err.Error(), options.Password) || values != nil {
					t.Fatal("rejected authentication did not return a sanitized setup error")
				}
			} else if err != nil || len(values) != 1 || values[0].Text != "PONG" ||
				len(values[0].Attributes) != 1 || values[0].Attributes[0].Value.Integer != 20 {
				t.Fatalf("expected only the command reply, got %+v, %v", values, err)
			}
			if err := <-peer; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRedisRESP3TypesAndMalformedFrames(t *testing.T) {
	read := func(input string) (RedisValue, error) {
		budget := 1024
		return readRedisValue(bufio.NewReader(strings.NewReader(input)), 0, &budget)
	}
	value, err := read("%?\r\n:7\r\n~?\r\n#f\r\n_\r\n.\r\n+numeric\r\n*3\r\n,inf\r\n,-1.25e-2\r\n(99999999999999999999\r\n+data\r\n*?\r\n$?\r\n;2\r\na\x00\r\n;1\r\n\xff\r\n;0\r\n=6\r\ntxt:hi\r\n!3\r\n\xffxx\r\n.\r\n.\r\n")
	if err != nil || value.Kind != "map" || len(value.Entries) != 3 {
		t.Fatalf("RESP3 map: %+v, %v", value, err)
	}
	key, set := value.Entries[0].Key, value.Entries[0].Value
	if key.Kind != "integer" || key.Integer != 7 || set.Kind != "set" || len(set.Items) != 2 ||
		set.Items[0].Kind != "boolean" || set.Items[0].Boolean == nil || *set.Items[0].Boolean || set.Items[1].Kind != "null" {
		t.Fatalf("RESP3 typed key and set: %+v, %+v", key, set)
	}
	numeric := value.Entries[1].Value.Items
	if len(numeric) != 3 || numeric[0].Kind != "double" || numeric[0].Text != "inf" ||
		numeric[1].Text != "-1.25e-2" || numeric[2].Kind != "big-number" || numeric[2].Text != "99999999999999999999" {
		t.Fatalf("RESP3 numbers: %+v", numeric)
	}
	data := value.Entries[2].Value.Items
	if len(data) != 3 || data[0].Kind != "bulk" || string(data[0].Data) != "a\x00\xff" ||
		data[1].Kind != "verbatim" || data[1].Format != "txt" || string(data[1].Data) != "hi" ||
		data[2].Kind != "bulk-error" || string(data[2].Data) != "\xffxx" {
		t.Fatalf("RESP3 binary replies: %+v", data)
	}
	for _, input := range []string{"#true\r\n", "_nil\r\n", ",0x1p0\r\n", "(1.1\r\n", "=4\r\ntext\r\n", "!-1\r\n",
		"%?\r\n+key\r\n.\r\n", "~?\r\n.bad\r\n", "$?\r\n;4294967295\r\n", "|0\r\n"} {
		if _, err := read(input); err == nil {
			t.Fatalf("accepted malformed RESP3 frame %q", input)
		}
	}
}

func TestRedisRESP3RejectsFailedNegotiation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	closed := make(chan bool, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			closed <- false
			return
		}
		defer connection.Close()
		_ = connection.SetDeadline(time.Now().Add(5 * time.Second))
		const hello = "*2\r\n$5\r\nHELLO\r\n$1\r\n3\r\n"
		data := make([]byte, len(hello))
		if _, err := io.ReadFull(connection, data); err != nil || string(data) != hello {
			closed <- false
			return
		}
		_, _ = io.WriteString(connection, "-ERR unsupported\r\n")
		_, err = connection.Read(data)
		closed <- err == io.EOF
	}()
	values, err := ExecuteRedisWithOptions(context.Background(), listener.Addr().String(), [][]string{{"PING"}}, 5*time.Second, RedisOptions{Protocol: 3})
	if err == nil || err.Error() != "Redis RESP3 negotiation failed" || values != nil || !<-closed {
		t.Fatal("failed RESP3 negotiation did not close before sending user commands")
	}
}

func TestRedisTLSVerifiesPeer(t *testing.T) {
	fixture := httptest.NewTLSServer(http.NotFoundHandler())
	defer fixture.Close()
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		MinVersion: tls.VersionTLS12, Certificates: fixture.TLS.Certificates,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			_ = connection.SetDeadline(time.Now().Add(5 * time.Second))
			data := make([]byte, len("*1\r\n$4\r\nPING\r\n"))
			if _, err := io.ReadFull(connection, data); err == nil {
				_, _ = io.WriteString(connection, "+PONG\r\n")
			}
			connection.Close()
		}
	}()
	execute := func(options RedisOptions) ([]RedisValue, error) {
		return ExecuteRedisWithOptions(context.Background(), listener.Addr().String(), [][]string{{"PING"}}, 5*time.Second, options)
	}
	options := RedisOptions{TLS: true}
	if _, err := execute(options); err == nil {
		t.Fatal("untrusted Redis certificate was accepted")
	}
	options.CAPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: fixture.Certificate().Raw}))
	options.ServerName = "wrong.invalid"
	if _, err := execute(options); err == nil {
		t.Fatal("wrong Redis certificate name was accepted")
	}
	options.ServerName = ""
	values, err := execute(options)
	if err != nil || len(values) != 1 || values[0].Text != "PONG" {
		t.Fatalf("trusted TLS exchange: %+v, %v", values, err)
	}
}

func TestRedisNetworkBinaryCommandAndError(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	observed := make(chan string, 1)
	const expected = "*2\r\n$4\r\nECHO\r\n$5\r\na\r\n\x00b\r\n"
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		_ = connection.SetDeadline(time.Now().Add(time.Second))
		data := make([]byte, len(expected))
		_, err = io.ReadFull(connection, data)
		if err == nil {
			observed <- string(data)
		}
		_, _ = io.WriteString(connection, "-ERR fixture\r\n")
	}()
	value, err := ExecuteRedis(context.Background(), listener.Addr().String(), []string{"ECHO", "a\r\n\x00b"}, time.Second)
	if err != nil || value.Kind != "error" || value.Text != "ERR fixture" {
		t.Fatalf("Redis error reply: %+v, %v", value, err)
	}
	select {
	case command := <-observed:
		if command != expected {
			t.Fatal("binary argument changed")
		}
	case <-time.After(time.Second):
		t.Fatal("command was not received")
	}
}

func TestRedisCancellationClosesConnection(t *testing.T) {
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
		buffer := make([]byte, 14)
		_, err = io.ReadFull(connection, buffer)
		if err != nil {
			closed <- false
			return
		}
		cancel()
		_, err = connection.Read(buffer)
		closed <- err == io.EOF
	}()
	_, err = ExecuteRedis(ctx, listener.Addr().String(), []string{"PING"}, time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel returned %v", err)
	}
	if !<-closed {
		t.Fatal("Redis connection remained open")
	}
}

func TestRedisPipeline(t *testing.T) {
	for _, fixture := range []struct {
		name      string
		replies   string
		oversized bool
	}{
		{name: "ordered replies after a Redis error", replies: "-ERR fixture\r\n+PONG\r\n"},
		{name: "shared response limit", replies: "+" + strings.Repeat("a", redisMessageLimit-7) + "\r\n+OK\r\n", oversized: true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			const expected = "*1\r\n$7\r\nUNKNOWN\r\n*1\r\n$4\r\nPING\r\n"
			received := make(chan string, 1)
			go func() {
				connection, err := listener.Accept()
				if err != nil {
					received <- ""
					return
				}
				defer connection.Close()
				_ = connection.SetDeadline(time.Now().Add(5 * time.Second))
				data := make([]byte, len(expected))
				_, err = io.ReadFull(connection, data)
				received <- string(data)
				if err == nil {
					_, _ = io.WriteString(connection, fixture.replies)
				}
			}()
			values, err := ExecuteRedisPipeline(context.Background(), listener.Addr().String(), [][]string{{"UNKNOWN"}, {"PING"}}, 5*time.Second)
			if fixture.oversized {
				if err == nil || !strings.Contains(err.Error(), "oversized Redis response") || values != nil {
					t.Fatalf("oversized response returned %d values, %v", len(values), err)
				}
			} else if err != nil || len(values) != 2 || values[0].Kind != "error" || values[1].Text != "PONG" {
				t.Fatalf("pipeline replies: %+v, %v", values, err)
			}
			if got := <-received; got != expected {
				t.Fatalf("received commands %q, want %q", got, expected)
			}
		})
	}
}

func TestRedisRESPTypesAndLimits(t *testing.T) {
	budget := 1024
	value, err := readRedisValue(bufio.NewReader(strings.NewReader("*4\r\n$3\r\n\x00\x01\xff\r\n-ERR fixture\r\n:-7\r\n$-1\r\n")), 0, &budget)
	if err != nil || len(value.Items) != 4 || string(value.Items[0].Data) != string([]byte{0, 1, 255}) || value.Items[1].Kind != "error" || value.Items[2].Integer != -7 || value.Items[3].Kind != "null" {
		t.Fatalf("RESP values: %+v, %v", value, err)
	}
	for _, input := range []string{"$99999\r\n", "+bad\n", "$2\r\na\r\n", "*10001\r\n"} {
		budget := 1024
		if _, err := readRedisValue(bufio.NewReader(strings.NewReader(input)), 0, &budget); err == nil {
			t.Fatalf("accepted malformed or oversized response %q", input)
		}
	}
}
