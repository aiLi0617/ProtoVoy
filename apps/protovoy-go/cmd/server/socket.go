package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"protovoy.dev/protovoy/internal/execution"
)

func validateHostedSocketTarget(target string) error {
	if target == "" {
		return nil
	}
	host, port, err := net.SplitHostPort(target)
	number, portErr := strconv.ParseUint(port, 10, 16)
	if err != nil || host == "" || portErr != nil || number == 0 || strings.ContainsAny(host, "@/\\?#\r\n\x00 \t") {
		return errors.New("socket target requires host:port with a port from 1 to 65535")
	}
	return nil
}

type socketTargets struct {
	ScheduleHTTPKeychain  bool
	AdditionalHTTP        map[string]struct{}
	TCP, UDP, Redis, GRPC string
	RedisOptions          execution.RedisOptions
	RedisPasswordRef      string
	MQTT                  *mqttEndpoint
	AMQP                  *amqpEndpoint
	SQL                   *sqlEndpoint
	Kafka                 *kafkaEndpoint
}

func socketHandler(origin string, targets socketTargets) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r, origin) || r.Header.Get("X-ProtoVoy-Preview") != "1" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var input execution.SocketPayload
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
			http.Error(w, "invalid socket request", http.StatusBadRequest)
			return
		}
		var target string
		switch input.Network {
		case "tcp":
			target = targets.TCP
		case "udp":
			target = targets.UDP
		default:
			http.Error(w, "select TCP or UDP", http.StatusBadRequest)
			return
		}
		if target == "" {
			http.Error(w, "socket endpoint is not configured", http.StatusServiceUnavailable)
			return
		}
		if input.ExpectedTarget != "" && input.ExpectedTarget != target {
			http.Error(w, "socket target mismatch", http.StatusConflict)
			return
		}
		if !input.ValidSendOptions() || len(input.Data) > (8<<20)/max(input.SendCount, 1) {
			http.Error(w, "invalid socket send options or repeated payload exceeds 8 MiB", http.StatusBadRequest)
			return
		}
		if !execution.ValidRequestTimeout(input.TimeoutMS) {
			http.Error(w, "invalid socket timeout", http.StatusBadRequest)
			return
		}
		if input.TimeoutMS > execution.DefaultRequestTimeoutMS {
			deadline := time.Now().Add(execution.RequestTimeout(input.TimeoutMS) + 5*time.Second)
			if err := http.NewResponseController(w).SetWriteDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
				http.Error(w, "could not prepare socket execution", http.StatusInternalServerError)
				return
			}
		}
		data, err := execution.ExchangeSocket(r.Context(), input.Request(target))
		if err != nil {
			if errors.Is(r.Context().Err(), context.Canceled) {
				http.Error(w, "socket execution canceled", http.StatusServiceUnavailable)
				return
			}
			if errors.Is(err, context.DeadlineExceeded) {
				http.Error(w, "socket execution timed out", http.StatusGatewayTimeout)
				return
			}
			http.Error(w, "socket exchange failed", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_ = json.NewEncoder(w).Encode(execution.SocketResponse{Data: base64.StdEncoding.EncodeToString(data)})
	}
}
