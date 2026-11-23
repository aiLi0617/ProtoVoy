package main

import (
	"io"
	"net"
	"testing"
	"time"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

func TestScheduledSocketExchangesAndTargetPolicy(t *testing.T) {
	for _, network := range []string{"tcp", "udp"} {
		t.Run(network, func(t *testing.T) {
			var target string
			if network == "tcp" {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer listener.Close()
				target = listener.Addr().String()
				go func() {
					conn, err := listener.Accept()
					if err != nil {
						return
					}
					defer conn.Close()
					conn.SetDeadline(time.Now().Add(5 * time.Second))
					data := make([]byte, 3)
					if _, err = io.ReadFull(conn, data); err == nil {
						conn.Write(data)
					}
				}()
			} else {
				conn, err := net.ListenPacket("udp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close()
				target = conn.LocalAddr().String()
				go func() {
					conn.SetDeadline(time.Now().Add(5 * time.Second))
					data := make([]byte, 32)
					n, peer, err := conn.ReadFrom(data)
					if err == nil {
						conn.WriteTo(data[:n], peer)
					}
				}()
			}
			execute := scheduledSocketExecutor(target, target)
			request := project.Request{URL: "{{target}}", Variables: map[string]string{"target": target}, Socket: &execution.SocketPayload{Network: network, Data: "AAH/", Encoding: "base64", Framing: "fixed", FixedLength: 3, TimeoutMS: 2000}}
			if network == "udp" {
				request.Socket.Framing = "raw"
				request.Socket.FixedLength = 0
			}
			result, err := execute(t.Context(), request)
			if err != nil || !result.Success {
				t.Fatalf("exchange: %+v %v", result, err)
			}
			for _, change := range []func(*project.Request){
				func(r *project.Request) { r.URL = "127.0.0.1:1" },
				func(r *project.Request) { r.Socket.SendCount = 10001 },
				func(r *project.Request) { r.Socket.Network = "unix" },
			} {
				input := request
				payload := *request.Socket
				input.Socket = &payload
				change(&input)
				if result, err = execute(t.Context(), input); err == nil || result.Success {
					t.Fatal("invalid socket request accepted")
				}
			}
		})
	}
}
