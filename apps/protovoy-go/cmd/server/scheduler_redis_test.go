package main

import (
	"io"
	"net"
	"testing"
	"time"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

func TestScheduledRedisReplyErrorsAndTargetPolicy(t *testing.T) {
	for _, reply := range []string{"+PONG\r\n", "-ERR secret diagnostic\r\n"} {
		t.Run(reply[:4], func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			done := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					done <- err
					return
				}
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(3 * time.Second))
				data := make([]byte, len("*1\r\n$4\r\nPING\r\n"))
				_, err = io.ReadFull(conn, data)
				if err == nil && string(data) != "*1\r\n$4\r\nPING\r\n" {
					t.Error("unexpected Redis command")
				}
				if err == nil {
					_, err = io.WriteString(conn, reply)
				}
				done <- err
			}()
			target := listener.Addr().String()
			execute := scheduledRedisExecutor(target, execution.RedisOptions{}, "")
			request := project.Request{URL: "{{target}}", Body: `[["{{command}}"]]`, Variables: map[string]string{"target": target, "command": "PING"}, BrokerTimeoutMS: 2000, Redis: &project.RedisSettings{Protocol: 2}}
			wrong := request
			wrong.URL = "127.0.0.1:1"
			if result, err := execute(t.Context(), wrong); err == nil || result.Success {
				t.Fatal("wrong target accepted")
			}
			result, err := execute(t.Context(), request)
			if reply[0] == '+' {
				if err != nil || !result.Success {
					t.Fatalf("PING: %+v %v", result, err)
				}
			} else if err == nil || result.Success || err.Error() != "Redis command returned an error" {
				t.Fatalf("Redis error accepted or leaked: %+v %v", result, err)
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Redis peer did not finish")
			}
		})
	}
}

func TestScheduledRedisBindingAssertions(t *testing.T) {
	options := execution.RedisOptions{TLS: true, Password: "fixture", Username: "reader", Database: "2", ServerName: "redis.test"}
	valid := project.RedisSettings{PasswordRef: "redis", Username: "reader", Database: "02", ServerName: "redis.test"}
	if err := validateScheduledRedisBinding(valid, options, "redis"); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*project.RedisSettings){
		func(s *project.RedisSettings) { s.PasswordRef = "wrong" },
		func(s *project.RedisSettings) { s.PasswordRef = "" },
		func(s *project.RedisSettings) { s.Username = "writer" },
		func(s *project.RedisSettings) { s.Database = "3" },
		func(s *project.RedisSettings) { s.ServerName = "other.test" },
	} {
		input := valid
		change(&input)
		if err := validateScheduledRedisBinding(input, options, "redis"); err == nil {
			t.Fatal("mismatched binding accepted")
		}
	}
	options.TLS = false
	if err := validateScheduledRedisBinding(valid, options, "redis"); err == nil {
		t.Fatal("credential binding allowed without TLS")
	}
}
