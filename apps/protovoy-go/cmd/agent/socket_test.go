package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"

	"protovoy.dev/protovoy/internal/agentclient"
	"protovoy.dev/protovoy/internal/execution"
)

func TestAgentSocketExchangesPreserveBinaryData(t *testing.T) {
	agent := testAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := agentclient.Pair(ctx, agent.Listener.Addr().String(), pairingFixture, agent.Client().Do)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("TCP fixed response over multiple events", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
		expected := bytes.Repeat([]byte{0, 1, 255}, 4000)
		observed := make(chan string, 1)
		go func() {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			defer connection.Close()
			_ = connection.SetDeadline(time.Now().Add(2 * time.Second))
			input := make([]byte, 3)
			if _, err := io.ReadFull(connection, input); err == nil {
				observed <- string(input)
				_, _ = connection.Write(append(append([]byte{}, expected...), 9, 9))
			}
		}()
		got, err := client.RunSocket(ctx, execution.SocketPayload{
			Network: "tcp", ExpectedTarget: "tcp://" + listener.Addr().String(),
			Encoding: "hex", Data: "00 01 ff", Framing: "fixed", FixedLength: len(expected),
		})
		if err != nil || !bytes.Equal(got, expected) || <-observed != "\x00\x01\xff" {
			t.Fatalf("TCP exchange changed payload or framing: %v (%d bytes)", err, len(got))
		}
	})
	t.Run("UDP empty datagram is a successful response", func(t *testing.T) {
		listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
		_ = listener.SetDeadline(time.Now().Add(2 * time.Second))
		go func() {
			var input [4]byte
			n, peer, err := listener.ReadFromUDP(input[:])
			if err == nil && n == 3 && bytes.Equal(input[:n], []byte{0, 1, 255}) {
				_, _ = listener.WriteToUDP([]byte{}, peer)
			}
		}()
		got, err := client.RunSocket(ctx, execution.SocketPayload{
			Network: "udp", ExpectedTarget: listener.LocalAddr().String(), Encoding: "base64", Data: "AAH/",
		})
		if err != nil || len(got) != 0 {
			t.Fatalf("empty UDP datagram failed: %v (%d bytes)", err, len(got))
		}
	})
}
