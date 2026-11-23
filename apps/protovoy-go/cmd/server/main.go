package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"protovoy.dev/protovoy/internal/buildinfo"
	"protovoy.dev/protovoy/internal/collaboration"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/scripting"
	"protovoy.dev/protovoy/internal/secrets"
)

const (
	webIndex  = "index.html"
	webLoader = "wasm_exec.js"
	webModule = "protovoy.wasm"
)

func main() {
	version := flag.Bool("version", false, "print the application version")
	listen := flag.String("listen", "127.0.0.1", "IP address to listen on")
	port := flag.Uint("port", 39300, "web server port")
	publicOrigin := flag.String("public-origin", "", "HTTPS origin for authenticated hosted mode")
	webPasswordFile := flag.String("web-password-file", "", "file containing the hosted Web password")
	tlsCert := flag.String("tls-cert", "", "PEM certificate chain for hosted mode")
	tlsKey := flag.String("tls-key", "", "PEM private key for hosted mode")
	web := flag.String("web", "build/web", "directory containing the Viva web build")
	teamDB := flag.String("team-db", "", "SQLite database for team collaboration (enables /api/team/)")
	scheduleDB := flag.String("schedule-db", "", "SQLite filesystem path for recurring jobs (enables /v1/jobs and /v1/executions)")
	scheduleHTTPKeychain := flag.Bool("schedule-http-keychain", false, "allow scheduled HTTP requests to use exact-target HTTP bindings from the system keychain")
	scheduleTimeout := flag.Duration("schedule-timeout", 30*time.Second, "maximum duration of each scheduled execution (up to one hour)")
	target := flag.String("target", "", "HTTP endpoint used by the request workbench")
	additionalHTTP := make(map[string]struct{})
	flag.Func("allow-http-target", "additional exact HTTP endpoint (repeatable, requires -target)", func(value string) error {
		if !validHostedHTTPTarget(value) {
			return errors.New("expected an absolute HTTP or HTTPS endpoint")
		}
		additionalHTTP[value] = struct{}{}
		return nil
	})
	socketTarget := flag.String("websocket-target", "", "WebSocket endpoint used by the request workbench")
	tcpTarget := flag.String("tcp-target", "", "allowed TCP host:port")
	udpTarget := flag.String("udp-target", "", "allowed UDP host:port")
	redisTarget := flag.String("redis-target", "", "allowed Redis host:port")
	grpcTarget := flag.String("grpc-target", "", "allowed HTTP/2 gRPC endpoint")
	mqttTarget := flag.String("mqtt-target", "", "allowed MQTT or MQTTS broker endpoint")
	mqttUsername := flag.String("mqtt-username", "", "MQTT username (password from "+execution.MQTTPasswordEnv+")")
	mqttServerName := flag.String("mqtt-server-name", "", "MQTTS certificate name and SNI")
	mqttCA := flag.String("mqtt-ca", "", "PEM CA file for MQTTS")
	amqpTarget := flag.String("amqp-target", "", "allowed AMQP or AMQPS broker and virtual host")
	amqpUsername := flag.String("amqp-username", "", "AMQP username (password from "+execution.AMQPPasswordEnv+")")
	amqpServerName := flag.String("amqp-server-name", "", "AMQPS certificate name and SNI")
	amqpCA := flag.String("amqp-ca", "", "PEM CA file for AMQPS")
	kafkaTarget := flag.String("kafka-target", "", "allowed Kafka broker addresses, comma-separated host:port")
	kafkaSecurity := flag.String("kafka-security", "PLAINTEXT", "Kafka security protocol: PLAINTEXT, SSL, or SASL_SSL")
	kafkaSASL := flag.String("kafka-sasl", "", "Kafka SASL mechanism: PLAIN, SCRAM-SHA-256, or SCRAM-SHA-512")
	kafkaUsername := flag.String("kafka-username", "", "Kafka SASL username (password from "+execution.KafkaPasswordEnv+")")
	kafkaPasswordKeychain := flag.Bool("kafka-password-keychain", false, "load the configured Kafka password reference from the system keychain")
	kafkaCA := flag.String("kafka-ca", "", "PEM CA file for Kafka TLS")
	kafkaCert := flag.String("kafka-client-cert", "", "PEM client certificate for Kafka mutual TLS")
	kafkaKey := flag.String("kafka-client-key", "", "PEM client key for Kafka mutual TLS")
	sqlTarget := flag.String("sql-target", "", "allowed PostgreSQL, MySQL or SQLite endpoint")
	sqlUsername := flag.String("sql-username", "", "SQL username (password from "+execution.SQLPasswordEnv+")")
	sqlPasswordKeychain := flag.Bool("sql-password-keychain", false, "load the configured SQL password reference from the system keychain")
	redisUsername := flag.String("redis-username", "", "Redis ACL username (password from "+execution.RedisPasswordEnv+")")
	redisDatabase := flag.String("redis-db", "", "Redis database number")
	redisTLS := flag.Bool("redis-tls", false, "enable verified TLS for Redis")
	redisServerName := flag.String("redis-server-name", "", "Redis TLS certificate name and SNI")
	redisCA := flag.String("redis-ca", os.Getenv("PROTOVOY_VIVA_REDIS_CA_FILE"), "PEM CA file for Redis TLS")
	flag.Parse()
	if *version {
		fmt.Println("ProtoVoy Web server " + buildinfo.Version)
		return
	}
	if *scheduleDB != "" {
		if err := validateScheduleStorage(*scheduleDB, *teamDB, *sqlTarget); err != nil {
			log.Fatal(err)
		}
		if *scheduleTimeout <= 0 || *scheduleTimeout > time.Hour {
			log.Fatal("-schedule-timeout must be positive and at most one hour")
		}
	}
	if *scheduleHTTPKeychain && *scheduleDB == "" {
		log.Fatal("-schedule-http-keychain requires -schedule-db")
	}
	if err := addHostedHTTPTargets(additionalHTTP, os.Getenv("PROTOVOY_HTTP_ALLOWED_TARGETS")); err != nil {
		log.Fatal(err)
	}
	if *grpcTarget != "" {
		if _, err := execution.ParseGRPCEndpoint(*grpcTarget); err != nil {
			log.Fatal(err)
		}
	}
	if (*target != "" || len(additionalHTTP) != 0) && !validHostedHTTPTarget(*target) {
		log.Fatal("set -target to an HTTP or HTTPS endpoint")
	}
	if *socketTarget != "" && !execution.ValidWebSocketTarget(*socketTarget) {
		log.Fatal("set -websocket-target to a ws:// or wss:// endpoint")
	}
	for _, endpoint := range []string{*tcpTarget, *udpTarget} {
		if err := validateHostedSocketTarget(endpoint); err != nil {
			log.Fatal(err)
		}
	}
	if *port == 0 || *port > 65535 {
		log.Fatal("set -port to a value between 1 and 65535")
	}
	redisOptions := execution.RedisOptions{
		Username: *redisUsername, Password: os.Getenv(execution.RedisPasswordEnv), Database: *redisDatabase,
		TLS: *redisTLS, ServerName: *redisServerName,
	}
	redisPasswordRef := os.Getenv(execution.RedisPasswordRefEnv)
	if err := validateHostedRedisTarget(*redisTarget, redisOptions.Database); err != nil {
		log.Fatal(err)
	}
	if *redisTarget != "" {
		if err := execution.ValidateRedisSetup(redisOptions); err != nil {
			log.Fatal(err)
		}
	}
	if *redisTarget != "" && redisPasswordRef != "" && (!execution.ValidPasswordReference(redisPasswordRef) || !redisOptions.TLS || redisOptions.Password == "") {
		log.Fatal("hosted Redis password references require a valid name, TLS and a configured password")
	}
	if *redisCA != "" {
		if *redisTarget == "" || !redisOptions.TLS {
			log.Fatal("Redis CA configuration requires -redis-target and -redis-tls")
		}
		ca, err := loadRedisCA(*redisCA)
		if err != nil {
			log.Fatal(err)
		}
		redisOptions.CAPEM = ca
	}
	var mqtt *mqttEndpoint
	if *mqttTarget != "" {
		var ca []byte
		if *mqttCA != "" {
			var err error
			ca, err = execution.ReadCAFile(*mqttCA)
			if err != nil || len(ca) == 0 {
				log.Fatal("could not read a nonempty MQTT CA file within 1 MiB")
			}
		}
		broker, err := execution.NewMQTTBroker(*mqttTarget, *mqttServerName, string(ca))
		if err != nil {
			log.Fatal(err)
		}
		mqtt = &mqttEndpoint{target: *mqttTarget, broker: broker, username: *mqttUsername, password: os.Getenv(execution.MQTTPasswordEnv), passwordRef: os.Getenv(execution.MQTTPasswordRefEnv)}
		if err := validateMQTTPasswordReference(mqtt); err != nil {
			log.Fatal(err)
		}
	}
	var amqp *amqpEndpoint
	if *amqpTarget != "" {
		var ca []byte
		if *amqpCA != "" {
			var err error
			ca, err = execution.ReadCAFile(*amqpCA)
			if err != nil || len(ca) == 0 {
				log.Fatal("could not read a nonempty AMQP CA file within 1 MiB")
			}
		}
		broker, err := execution.NewAMQPBroker(*amqpTarget, *amqpUsername, os.Getenv(execution.AMQPPasswordEnv), *amqpServerName, string(ca))
		if err != nil {
			log.Fatal(err)
		}
		reference := os.Getenv(execution.AMQPPasswordRefEnv)
		if err := validateAMQPPasswordReference(*amqpTarget, *amqpUsername, os.Getenv(execution.AMQPPasswordEnv), reference); err != nil {
			log.Fatal(err)
		}
		amqp = &amqpEndpoint{target: *amqpTarget, broker: broker, passwordRef: reference}
	}
	kafkaPassword := os.Getenv(execution.KafkaPasswordEnv)
	kafkaReference := os.Getenv(execution.KafkaPasswordRefEnv)
	var kafkaPasswordLookup func(string) (string, error)
	if *kafkaPasswordKeychain {
		if *kafkaTarget == "" {
			log.Fatal("-kafka-password-keychain requires -kafka-target")
		}
		kafkaPassword = ""
		kafkaPasswordLookup = secrets.Resolve
	}
	kafka, err := configureKafkaEndpoint(*kafkaTarget, execution.KafkaOptions{
		SecurityProtocol: *kafkaSecurity, SASLMechanism: *kafkaSASL, Username: *kafkaUsername,
		Password: kafkaPassword, PasswordRef: kafkaReference,
		KafkaTLSReferences: execution.KafkaTLSReferences{
			CAPEMRef: os.Getenv("PROTOVOY_KAFKA_CA_REF"), CertificatePEMRef: os.Getenv("PROTOVOY_KAFKA_CERT_REF"),
			KeyPEMRef: os.Getenv("PROTOVOY_KAFKA_KEY_REF"),
		},
	}, *kafkaCA, *kafkaCert, *kafkaKey, kafkaPasswordLookup)
	if err != nil {
		log.Fatal(err)
	}
	var sql *sqlEndpoint
	if *sqlPasswordKeychain && *sqlTarget == "" {
		log.Fatal("-sql-password-keychain requires -sql-target")
	}
	if *sqlTarget != "" {
		if err := checkTeamSQLIsolation(*teamDB, *sqlTarget); err != nil {
			log.Fatal(err)
		}
		password, reference := os.Getenv(execution.SQLPasswordEnv), os.Getenv(execution.SQLPasswordRefEnv)
		var binding execution.SQLPasswordBinding
		var err error
		if *sqlPasswordKeychain {
			binding, err = execution.ResolveSQLPasswordBinding(reference, *sqlTarget, *sqlUsername, secrets.Resolve)
		} else {
			binding, err = execution.NewSQLPasswordBinding(reference, *sqlTarget, *sqlUsername, password)
		}
		if err != nil {
			log.Fatal(err)
		}
		credentials := execution.SQLCredentials{Username: *sqlUsername, PasswordRef: reference}
		if reference == "" {
			credentials.Password = password
		}
		database, err := binding.Resolve(*sqlTarget, credentials)
		if err != nil {
			log.Fatal(err)
		}
		sql = &sqlEndpoint{target: *sqlTarget, database: database, passwordRef: reference}
	}
	addr := net.JoinHostPort(*listen, strconv.Itoa(int(*port)))
	password, err := loadWebPassword(os.Getenv(webPasswordEnv), *webPasswordFile)
	if err != nil {
		log.Fatal(err)
	}
	access, err := configureWebAccess(addr, *publicOrigin, *tlsCert, *tlsKey, os.Getenv(webUserEnv), password)
	if err != nil {
		log.Fatal(err)
	}
	if err := checkWebBuild(*web); err != nil {
		log.Fatal(err)
	}
	targets := socketTargets{
		ScheduleHTTPKeychain: *scheduleHTTPKeychain,
		TCP:                  *tcpTarget, UDP: *udpTarget, Redis: *redisTarget, RedisOptions: redisOptions, RedisPasswordRef: redisPasswordRef, GRPC: *grpcTarget,
		MQTT: mqtt, AMQP: amqp, SQL: sql, Kafka: kafka, AdditionalHTTP: additionalHTTP,
	}
	webHandler := handler(access, *web, *target, *socketTarget, targets)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *teamDB != "" {
		oidcConfig := collaboration.OIDCConfig{
			Issuer: os.Getenv("PROTOVOY_TEAM_OIDC_ISSUER"), ClientID: os.Getenv("PROTOVOY_TEAM_OIDC_CLIENT_ID"),
			ClientSecret: os.Getenv("PROTOVOY_TEAM_OIDC_CLIENT_SECRET"), Organization: os.Getenv("PROTOVOY_TEAM_OIDC_ORGANIZATION_ID"),
			Role: os.Getenv("PROTOVOY_TEAM_OIDC_DEFAULT_ROLE"),
		}
		team, closeTeam, err := openTeamHandler(ctx, access, *teamDB, os.Getenv(teamBootstrapEnv), oidcConfig)
		if err != nil {
			log.Fatal(err)
		}
		defer closeTeam()
		mux := http.NewServeMux()
		mux.Handle("/api/team/", team)
		mux.Handle("/", webHandler.Handler)
		webHandler.Handler = mux
	}
	closeSchedules := func() {}
	if *scheduleDB != "" {
		schedules, closeService, err := openScheduledService(ctx, access, *scheduleDB, *scheduleTimeout,
			scheduledExecutor(*target, *socketTarget, targets), func(err error) { log.Print(err) })
		if err != nil {
			log.Fatal("could not initialize schedule service")
		}
		closeSchedules = closeService
		defer closeSchedules()
		mux := http.NewServeMux()
		mux.Handle("/v1/jobs", schedules)
		mux.Handle("/v1/jobs/", schedules)
		mux.Handle("/v1/executions", schedules)
		mux.Handle("/", webHandler.Handler)
		webHandler.Handler = mux
	}
	server := &http.Server{
		Addr:              addr,
		Handler:           webHandler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      35 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	log.Printf("Serving ProtoVoy Web for %s on %s", access.origin, addr)
	serve := server.ListenAndServe
	if access.auth != nil {
		server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		serve = func() error { return server.ListenAndServeTLS(*tlsCert, *tlsKey) }
	}
	if err := serveUntilShutdown(ctx, server, serve, webHandler.bridge.stop); err != nil {
		closeSchedules()
		log.Fatal(err)
	}
}

func addHostedHTTPTargets(targets map[string]struct{}, lines string) error {
	for line := range strings.SplitSeq(lines, "\n") {
		value := strings.TrimSpace(line)
		if value == "" {
			continue
		}
		if !validHostedHTTPTarget(value) {
			return errors.New("PROTOVOY_HTTP_ALLOWED_TARGETS requires one absolute HTTP or HTTPS endpoint per line")
		}
		targets[value] = struct{}{}
	}
	return nil
}

func validHostedHTTPTarget(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Hostname() != "" && parsed.User == nil && parsed.Fragment == "" &&
		(parsed.Scheme == "http" || parsed.Scheme == "https")
}

func serveUntilShutdown(ctx context.Context, server *http.Server, serve func() error, stopBridge func() <-chan struct{}) error {
	done := make(chan error, 1)
	go func() { done <- serve() }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		upgrades := stopBridge()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return fmt.Errorf("could not drain HTTP requests: %w", err)
		}
		if err := <-done; !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		select {
		case <-upgrades:
			return nil
		case <-shutdownCtx.Done():
			return fmt.Errorf("could not drain WebSocket connections: %w", shutdownCtx.Err())
		}
	}
}

func checkWebBuild(dir string) error {
	for _, asset := range []string{webIndex, webLoader, webModule} {
		path := filepath.Join(dir, asset)
		entry, err := os.Stat(path)
		if err != nil || !entry.Mode().IsRegular() || entry.Size() == 0 {
			return fmt.Errorf("the web build requires a nonempty %s file", asset)
		}
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("could not read %s from the web build: %w", asset, err)
		}
		file.Close()
	}
	return nil
}

type webApplication struct {
	http.Handler
	bridge *webSocketBridge
}

func handler(access webAccess, web, target, socketTarget string, sockets ...socketTargets) *webApplication {
	scriptSlots := make(chan struct{}, 2)
	bridge := &webSocketBridge{stopping: make(chan struct{})}
	mux := http.NewServeMux()
	serveAsset := func(name string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Viva uses fixed asset names across releases.
			w.Header().Set("Cache-Control", "no-store")
			if name == webModule {
				w.Header().Set("Content-Type", "application/wasm")
			}
			http.ServeFile(w, r, filepath.Join(web, name))
		}
	}
	mux.HandleFunc("GET /{$}", serveAsset(webIndex))
	mux.HandleFunc("GET /"+webLoader, serveAsset(webLoader))
	mux.HandleFunc("GET /"+webModule, serveAsset(webModule))
	grpcTarget := ""
	var mqtt *mqttEndpoint
	var amqp *amqpEndpoint
	var sql *sqlEndpoint
	var kafka *kafkaEndpoint
	var additionalHTTP map[string]struct{}
	if len(sockets) > 0 {
		additionalHTTP = sockets[0].AdditionalHTTP
		grpcTarget = sockets[0].GRPC
		mqtt = sockets[0].MQTT
		amqp = sockets[0].AMQP
		sql = sockets[0].SQL
		kafka = sockets[0].Kafka
		mux.HandleFunc("POST /api/socket", stopStreamOnShutdown(bridge.stopping, socketHandler(access.origin, sockets[0])))
		mux.HandleFunc("POST "+execution.RedisBridgePath, stopStreamOnShutdown(bridge.stopping, redisHandler(access.origin, sockets[0].Redis, sockets[0].RedisOptions, sockets[0].RedisPasswordRef)))
	}
	mux.HandleFunc("POST "+execution.GRPCBridgePath, grpcHandler(access.origin, grpcTarget, bridge.stopping))
	mux.HandleFunc("POST "+execution.MQTTBridgePath, stopStreamOnShutdown(bridge.stopping, mqttHandler(access.origin, mqtt)))
	mux.HandleFunc("POST "+execution.AMQPBridgePath, stopStreamOnShutdown(bridge.stopping, amqpHandler(access.origin, amqp)))
	mux.HandleFunc("POST "+execution.SQLBridgePath, stopStreamOnShutdown(bridge.stopping, sqlHandler(access.origin, sql)))
	mux.HandleFunc("POST "+execution.KafkaBridgePath, stopStreamOnShutdown(bridge.stopping, kafkaHandler(access.origin, kafka)))
	mux.HandleFunc("GET "+execution.WebSocketBridgePath, bridge.handler(access.origin, socketTarget))
	mux.HandleFunc("POST /api/preview", func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r, access.origin) || r.Header.Get("X-ProtoVoy-Preview") != "1" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if target == "" {
			http.Error(w, "HTTP endpoint is not configured", http.StatusServiceUnavailable)
			return
		}
		request, ok := decodePayload(w, r)
		if !ok {
			return
		}
		upstreamTarget := target
		if request.ExpectedTarget != "" && request.ExpectedTarget != target {
			if _, allowed := additionalHTTP[request.ExpectedTarget]; !allowed {
				http.Error(w, "target does not match a configured endpoint", http.StatusConflict)
				return
			}
			upstreamTarget = request.ExpectedTarget
		}
		hasScripts := len(request.PreScripts) != 0 || len(request.PostScripts) != 0
		if hasScripts {
			select {
			case scriptSlots <- struct{}{}:
				defer func() { <-scriptSlots }()
			default:
				http.Error(w, "hosted script capacity is busy", http.StatusTooManyRequests)
				return
			}
		}
		payload, err := request.HTTPPayload.EncodeBody()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		controller := http.NewResponseController(w)
		if payload.TimeoutMS > execution.DefaultRequestTimeoutMS {
			deadline := time.Now().Add(time.Duration(payload.TimeoutMS)*time.Millisecond + 5*time.Second)
			if err := controller.SetWriteDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
				http.Error(w, "could not prepare HTTP execution", http.StatusInternalServerError)
				return
			}
		}
		ctx := r.Context()
		if payload.EventStream {
			streamContext, cancel := streamShutdownContext(ctx, bridge.stopping)
			defer cancel()
			ctx = streamContext
		}
		input := payload.Request(upstreamTarget)
		var scriptData json.RawMessage
		variables := request.Variables
		if hasScripts {
			input, scriptData, variables, err = scripting.PrepareHTTP(ctx, input, request.PreScripts, variables)
			if err != nil {
				w.Header().Set("X-ProtoVoy-Script-Phase", "pre-request")
				http.Error(w, "pre-request script failed", http.StatusUnprocessableEntity)
				return
			}
			if input.Target != target {
				if _, allowed := additionalHTTP[input.Target]; !allowed {
					http.Error(w, "script target does not match a configured endpoint", http.StatusConflict)
					return
				}
			}
			if input.FollowRedirects || !validHostedHTTPMethod(input.Method) {
				http.Error(w, "script produced an unsupported method or redirect policy", http.StatusBadRequest)
				return
			}
		}
		started := time.Now()
		response, err := (execution.HTTPClient{}).Execute(ctx, input)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				http.Error(w, "request timed out", http.StatusGatewayTimeout)
				return
			}
			http.Error(w, "upstream request failed", http.StatusBadGateway)
			return
		}
		defer response.Body.Close()
		if len(request.PostScripts) != 0 {
			body, err := scripting.ReadHTTPPost(ctx, response, request.PostScripts, scriptData, variables, started)
			if err != nil {
				w.Header().Set("X-ProtoVoy-Script-Phase", "post-response")
				http.Error(w, "post-response script failed", http.StatusUnprocessableEntity)
				return
			}
			response.Body = io.NopCloser(bytes.NewReader(body))
		}
		contentType := response.Headers.Get("Content-Type")
		eventStream := request.EventStream && execution.IsEventStream(response.StatusCode, contentType)
		w.Header().Set("X-ProtoVoy-Status", strconv.Itoa(response.StatusCode))
		w.Header().Set("X-ProtoVoy-Content-Type", contentType)
		if preview := execution.PreviewResponseHeaders(response.Headers); len(preview) != 0 {
			data, _ := json.Marshal(preview)
			if base64.StdEncoding.EncodedLen(len(data)) <= 8192 {
				w.Header().Set(execution.ResponseHeadersMetadata, base64.StdEncoding.EncodeToString(data))
			}
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		stream := streamWriter{ResponseWriter: w, controller: controller, eventStream: eventStream}
		if err := stream.withWriteDeadline(stream.controller.Flush); err != nil {
			panic(http.ErrAbortHandler)
		}
		// A read failure after headers are sent must abort the response rather
		// than present a partial body as a successful preview.
		var body io.Reader = response.Body
		if !eventStream {
			body = io.LimitReader(body, execution.PreviewLimit+1)
		}
		if _, err := io.Copy(stream, body); err != nil {
			panic(http.ErrAbortHandler)
		}
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if err := checkWebBuild(web); err != nil {
			http.Error(w, "workbench assets unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return &webApplication{bridge: bridge, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !access.authorize(w, r) {
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		mux.ServeHTTP(w, r)
	})}
}

func stopStreamOnShutdown(stopping <-chan struct{}, handle http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := streamShutdownContext(r.Context(), stopping)
		defer cancel()
		handle(w, r.WithContext(ctx))
	}
}

var errServerStopping = errors.New("server is shutting down")

func streamShutdownContext(parent context.Context, stopping <-chan struct{}) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(parent)
	go func() {
		select {
		case <-stopping:
			cancel(errServerStopping)
		case <-ctx.Done():
		}
	}()
	return ctx, func() { cancel(nil) }
}

func decodePayload(w http.ResponseWriter, r *http.Request) (execution.PreviewRequest, bool) {
	var payload execution.PreviewRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&payload)
	if err == nil {
		err = decoder.Decode(new(any))
		if err == io.EOF {
			if err := validateHostedScripts(payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return payload, false
			}
			if payload.FollowRedirects {
				http.Error(w, "hosted HTTP execution cannot follow redirects", http.StatusBadRequest)
				return payload, false
			}
			if !execution.ValidRequestTimeout(payload.TimeoutMS) || payload.EventStream && payload.TimeoutMS != 0 {
				http.Error(w, "invalid HTTP timeout", http.StatusBadRequest)
				return payload, false
			}
			if payload.EventStream && (payload.Method != http.MethodGet || payload.Body != "") {
				http.Error(w, "SSE requires GET without a body", http.StatusBadRequest)
				return payload, false
			}
			switch payload.Method {
			case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
				http.MethodPatch, http.MethodDelete, http.MethodOptions:
				return payload, true
			}
		}
	}
	var limit *http.MaxBytesError
	if errors.As(err, &limit) {
		http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
	} else {
		http.Error(w, "invalid HTTP request", http.StatusBadRequest)
	}
	return payload, false
}

type streamWriter struct {
	http.ResponseWriter
	controller  *http.ResponseController
	eventStream bool
}

func (stream streamWriter) Write(data []byte) (int, error) {
	var n int
	err := stream.withWriteDeadline(func() error {
		var err error
		n, err = stream.ResponseWriter.Write(data)
		if err != nil {
			return err
		}
		return stream.controller.Flush()
	})
	return n, err
}

func (stream streamWriter) withWriteDeadline(write func() error) error {
	if stream.eventStream {
		if err := stream.controller.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
			return err
		}
		defer stream.controller.SetWriteDeadline(time.Time{})
	}
	return write()
}
