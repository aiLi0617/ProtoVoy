package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"protovoy.dev/protovoy/internal/agentclient"
	"protovoy.dev/protovoy/internal/execution"
)

func TestAgentRedisPreservesPipelineRepliesAndLiteralArguments(t *testing.T) {
	agent := testAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := agentclient.Pair(ctx, agent.Listener.Addr().String(), pairingFixture, agent.Client().Do)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	data := bytes.Repeat([]byte{0, 1, 255}, 4000)
	peer := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			peer <- err
			return
		}
		defer connection.Close()
		_ = connection.SetDeadline(time.Now().Add(4 * time.Second))
		for _, frame := range []struct{ request, response string }{
			{"*3\r\n$4\r\nAUTH\r\n$4\r\nuser\r\n$7\r\nfixture\r\n", "+OK\r\n"},
			{"*2\r\n$5\r\nHELLO\r\n$1\r\n3\r\n", "%1\r\n+proto\r\n:3\r\n"},
			{"*2\r\n$6\r\nSELECT\r\n$1\r\n2\r\n", "+OK\r\n"},
			{"*2\r\n$4\r\nECHO\r\n$8\r\n{{keep}}\r\n*1\r\n$7\r\nUNKNOWN\r\n", fmt.Sprintf("$%d\r\n%s\r\n-ERR fixture\r\n", len(data), data)},
		} {
			request := make([]byte, len(frame.request))
			if _, err := io.ReadFull(connection, request); err != nil || string(request) != frame.request {
				peer <- errors.New("unexpected Redis command")
				return
			}
			if _, err := io.WriteString(connection, frame.response); err != nil {
				peer <- err
				return
			}
		}
		peer <- nil
	}()
	values, err := client.RunRedis(ctx, execution.RedisPayload{
		ExpectedTarget: "redis://" + listener.Addr().String(), Protocol: 3,
		Commands: [][]string{{"ECHO", "{{keep}}"}, {"UNKNOWN"}},
	}, execution.RedisOptions{Username: "user", Password: "fixture", Database: "2"})
	if err != nil || len(values) != 2 || values[0].Kind != "bulk" || !bytes.Equal(values[0].Data, data) || values[1].Kind != "error" {
		t.Fatalf("invalid Redis replies: %d values, %v", len(values), err)
	}
	if err := <-peer; err != nil {
		t.Fatal(err)
	}
}

func TestRedisAgentTargetDisallowsURLCredentials(t *testing.T) {
	for _, target := range []string{"redis://user:password@localhost:6379", "rediss://localhost/2", "http://localhost:6379", "localhost:6379?arg=value"} {
		if _, _, err := redisTarget(target); err == nil {
			t.Fatal("accepted unsupported Redis URL")
		}
	}
	if addr, secure, err := redisTarget("rediss://[::1]"); err != nil || !secure || addr != "[::1]:6379" {
		t.Fatalf("unexpected Redis TLS endpoint: %q, %v", addr, err)
	}
}
