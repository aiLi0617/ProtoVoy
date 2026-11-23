package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"protovoy.dev/protovoy/internal/agenthistory"
	"protovoy.dev/protovoy/internal/buildinfo"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/plugins"
	"protovoy.dev/protovoy/internal/secrets"
)

const apiVersion = "1"

var agentVersion = buildinfo.Version

const versionHeader = "X-ProtoVoy-Protocol-Api-Version"

func main() {
	version := flag.Bool("version", false, "print the application version")
	bind := flag.String("bind", envDefault("PROTOVOY_AGENT_BIND", "127.0.0.1:39217"), "loopback IP:port")
	origins := flag.String("allowed-origins", envDefault("PROTOVOY_ALLOWED_ORIGINS", "http://127.0.0.1:39300,http://localhost:5180,http://tauri.localhost,tauri://localhost"), "comma-separated exact browser origins")
	tokenFile := flag.String("token-file", "", "file containing an existing pairing token")
	historyDir := flag.String("history-dir", "", "directory for Local Agent execution history")
	sqliteTarget := flag.String("sqlite-target", "", "allowed SQLite file URL for paired clients")
	sqlPasswordKeychain := flag.Bool("sql-password-keychain", false, "load the configured SQL password reference from the system keychain")
	kafkaPasswordKeychain := flag.Bool("kafka-password-keychain", false, "resolve the configured Kafka password reference from the system keychain")
	mqttPasswordKeychain := flag.Bool("mqtt-password-keychain", false, "resolve the configured MQTT password reference from the system keychain")
	amqpPasswordKeychain := flag.Bool("amqp-password-keychain", false, "resolve the configured AMQP password reference from the system keychain")
	redisPasswordKeychain := flag.Bool("redis-password-keychain", false, "resolve the configured Redis password reference from the system keychain")
	historyResponseBytes := flag.Int("history-response-bytes", 0, "maximum response bytes saved per execution (0 disables capture)")
	flag.Parse()
	if *version {
		fmt.Println("ProtoVoy Local Agent " + buildinfo.Version)
		return
	}
	sqlPasswords, err := loadAgentSQLPasswordBinding(*sqlPasswordKeychain)
	if err != nil {
		log.Fatal(err)
	}
	if *sqliteTarget != "" {
		if !strings.HasPrefix(*sqliteTarget, "sqlite:") || *sqliteTarget == execution.SQLiteMemoryTarget {
			log.Fatal("set -sqlite-target to a SQLite file URL")
		}
		if _, err := execution.NewSQLEndpoint(*sqliteTarget, "", ""); err != nil {
			log.Fatal(err)
		}
	}
	if *historyResponseBytes < 0 || *historyResponseBytes > agenthistory.MaxResponseBytes {
		log.Fatal("set -history-response-bytes between 0 and 1048576")
	}
	addr, err := netip.ParseAddrPort(*bind)
	if err != nil || !addr.Addr().IsLoopback() || addr.Addr().Zone() != "" {
		log.Fatal("the Local Agent bind address must use a loopback IP and port")
	}
	token := os.Getenv("PROTOVOY_AGENT_TOKEN")
	if token == "" && *tokenFile != "" {
		token, err = readTokenFile(*tokenFile)
		if err != nil {
			log.Fatal(err)
		}
	}
	listener, err := net.Listen("tcp", addr.String())
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()
	history, err := openHistory(*historyDir)
	if err != nil {
		log.Fatal("could not open Local Agent history")
	}
	defer history.root.Close()
	history.responseLimit = *historyResponseBytes
	service, err := newAgent(listener.Addr().String(), token, *origins, history)
	if err != nil {
		log.Fatal(err)
	}
	service.sqliteTarget = *sqliteTarget
	service.pluginDirectory = os.Getenv("PROTOVOY_PLUGIN_DIR")
	service.pluginKeys, err = plugins.ParseTrustedKeys(os.Getenv("PROTOVOY_PLUGIN_TRUSTED_KEYS"))
	if err != nil {
		log.Fatal(err)
	}
	service.sqlPasswords = sqlPasswords
	service.kafkaPasswordKeychain = *kafkaPasswordKeychain
	service.mqttPasswordKeychain = *mqttPasswordKeychain
	service.amqpPasswordKeychain = *amqpPasswordKeychain
	service.redisPasswordKeychain = *redisPasswordKeychain
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{
		Handler: service.routes(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second,
		IdleTimeout: 30 * time.Second, MaxHeaderBytes: 32 << 10,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	log.Printf("ProtoVoy Go Local Agent listening on http://%s", listener.Addr())
	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	select {
	case err := <-result:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	case <-ctx.Done():
		deadline, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if err := server.Shutdown(deadline); err != nil {
			_ = server.Close()
		}
		service.mockMu.Lock()
		if service.mockRuntime != nil {
			mockDeadline, mockCancel := context.WithTimeout(context.Background(), 6*time.Second)
			err = service.mockRuntime.Close(mockDeadline)
			mockCancel()
			service.mockRuntime = nil
		}
		service.mockMu.Unlock()
		if err != nil {
			log.Fatal("could not finish Mock shutdown")
		}
		if err := <-result; !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}
}

func loadAgentSQLPasswordBinding(useKeychain bool) (execution.SQLPasswordBinding, error) {
	if !useKeychain {
		return execution.LoadSQLPasswordBinding()
	}
	return execution.ResolveSQLPasswordBinding(os.Getenv(execution.SQLPasswordRefEnv),
		os.Getenv(execution.SQLTargetEnv), os.Getenv(execution.SQLUsernameEnv), secrets.Resolve)
}

func envDefault(name, fallback string) string {
	if value, exists := os.LookupEnv(name); exists {
		return value
	}
	return fallback
}

func readTokenFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", errors.New("could not open Local Agent pairing token file")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 1025))
	if err != nil || len(data) > 1024 {
		return "", errors.New("could not read Local Agent pairing token file within 1 KiB")
	}
	return strings.TrimSpace(string(data)), nil
}

func (agent *agentService) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"service": "protovoy-agent", "version": agentVersion, "agentVersion": agentVersion,
			"bind": agent.addr, "protocolApiVersion": apiVersion, "minProtocolApiVersion": apiVersion,
			"maxProtocolApiVersion": apiVersion, "authRequired": true,
			"workspaceApiVersion": "1",
			"mockApiVersion":      "1",
			"secretsApiVersion":   "1",
			"pluginsApiVersion":   "1",
			"pluginUpdates":       true,
		})
	})
	mux.HandleFunc("POST /v1/session", agent.createSession)
	mux.HandleFunc("GET /v1/capabilities", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, []map[string]any{{
			"protocolId": "http", "version": agentVersion, "displayName": "HTTP / HTTPS",
			"capabilities": []string{"streaming", "tls", "binary"},
		}, {
			"protocolId": "sse", "version": agentVersion, "displayName": "Server-Sent Events",
			"capabilities": []string{"streaming", "reconnect-token", "tls"},
		}, {
			"protocolId": "tcp", "version": agentVersion, "displayName": "TCP",
			"capabilities": []string{"binary", "delimiter-framing", "fixed-framing", "tls"},
		}, {
			"protocolId": "udp", "version": agentVersion, "displayName": "UDP",
			"capabilities": []string{"binary"},
		}, {
			"protocolId": "redis", "version": agentVersion, "displayName": "Redis RESP2 / RESP3",
			"capabilities": []string{"binary", "pipeline", "tls"},
		}, {
			"protocolId": "mqtt", "version": agentVersion, "displayName": "MQTT 3.1.1",
			"capabilities": []string{"publish", "subscribe", "qos0", "qos1", "qos2", "tls", "auth", "retain", "cancel"},
		}, {
			"protocolId": "amqp", "version": agentVersion, "displayName": "AMQP 0-9-1",
			"capabilities": []string{"publish", "consume", "tls", "publisher-confirms", "manual-ack", "topology"},
		}, {
			"protocolId": "kafka", "version": agentVersion, "displayName": "Apache Kafka",
			"capabilities": []string{"produce", "consume", "consumer-groups", "manual-commit", "tls", "sasl"},
		}, {
			"protocolId": "sql", "version": agentVersion, "displayName": "SQL (PostgreSQL / MySQL / SQLite)",
			"capabilities": []string{"postgresql", "mysql", "sqlite", "parameters", "transactions", "tls", "row-limit"},
		}, {
			"protocolId": "grpc", "version": agentVersion, "displayName": "gRPC",
			"capabilities": []string{"binary", "unary", "server-streaming", "client-streaming", "bidi-streaming", "http2"},
		}, {
			"protocolId": "jsonrpc", "version": agentVersion, "displayName": "JSON-RPC 2.0",
			"capabilities": []string{"http-transport", "headers", "tls", "cancel"},
		}, {
			"protocolId": "soap", "version": agentVersion, "displayName": "SOAP 1.1 / 1.2",
			"capabilities": []string{"http-transport", "headers", "tls", "cancel"},
		}})
	})
	mux.HandleFunc("POST /v1/executions", agent.startExecution)
	mux.HandleFunc("GET /v1/executions/{id}/events", agent.executionEvents)
	mux.HandleFunc("POST /v1/executions/{id}/cancel", agent.cancelExecution)
	mux.HandleFunc("GET /v1/history", agent.listHistory)
	mux.HandleFunc("GET /v1/mock", agent.manageMock)
	mux.HandleFunc("GET /v1/secrets/{reference}", agent.secret)
	mux.HandleFunc("GET /v1/plugins", agent.listPlugins)
	mux.HandleFunc("POST /v1/plugins", agent.installPlugin)
	mux.HandleFunc("POST /v1/plugins/{id}/update", agent.installPlugin)
	mux.HandleFunc("POST /v1/plugins/{id}/run", agent.runPlugin)
	mux.HandleFunc("POST /v1/plugins/{id}/state", agent.setPluginState)
	mux.HandleFunc("DELETE /v1/plugins/{id}", agent.removePlugin)
	mux.HandleFunc("GET /v1/http-secrets/{reference}", agent.httpSecret)
	mux.HandleFunc("PUT /v1/http-secrets/{reference}", agent.httpSecret)
	mux.HandleFunc("DELETE /v1/http-secrets/{reference}", agent.httpSecret)
	mux.HandleFunc("PUT /v1/secrets/{reference}", agent.secret)
	mux.HandleFunc("DELETE /v1/secrets/{reference}", agent.secret)
	mux.HandleFunc("POST /v1/mock", agent.manageMock)
	mux.HandleFunc("DELETE /v1/mock", agent.manageMock)
	mux.HandleFunc("GET /v1/workspace", agent.workspace)
	mux.HandleFunc("PUT /v1/workspace", agent.workspace)
	mux.HandleFunc("DELETE /v1/workspace", agent.workspace)
	mux.HandleFunc("GET /v1/history/{id}", agent.historyItem)
	mux.HandleFunc("GET /v1/history/{id}/body", agent.historyBody)
	mux.HandleFunc("POST "+execution.AgentWebSocketTicketPath, agent.createWebSocketTicket)
	mux.HandleFunc("GET "+execution.AgentWebSocketPath, agent.upgradeWebSocket)
	return agent.authorize(mux)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
