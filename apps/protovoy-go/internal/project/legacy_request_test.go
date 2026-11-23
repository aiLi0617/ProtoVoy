package project

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestReadLegacyHTTPRejectsInvalidOrSensitiveBodySource(t *testing.T) {
	for _, source := range []string{`not JSON`, `{\"password\":\"fixture\"}`} {
		input := `{"protocolId":"http","target":"http://localhost/","payload":{"type":"http","method":"POST","body":"{}","bodySource":"` + source + `"}}`
		if _, err := Read(strings.NewReader(input)); err == nil || strings.Contains(err.Error(), "fixture") {
			t.Fatal("expected a sanitized HTTP source import error")
		}
	}
}

func TestReadLegacyRequestsAndRoundTripAsPortableProject(t *testing.T) {
	for _, tc := range []struct {
		protocol, target, payload, wantBody string
		check                               func(Request) bool
	}{
		{"tcp", "tcp://localhost:9000", `{"type":"tcp","data":"ff","encoding":"hex","framing":"fixed","fixedLength":2,"sendCount":3,"intervalMs":10}`, "",
			func(r Request) bool {
				return r.URL == "localhost:9000" && r.Socket != nil && r.Socket.FixedLength == 2 &&
					r.Socket.SendCount == 3 && r.Socket.IntervalMS == 10 && r.Socket.Data == "ff"
			}},
		{"udp", "localhost:9001", `{"type":"udp","data":"hello","sendCount":2,"intervalMs":20}`, "",
			func(r Request) bool { return r.Socket != nil && r.Socket.Network == "udp" && r.Socket.SendCount == 2 }},
		{"sse", "https://example.com/events", `{"type":"sse","headers":[["Last-Event-ID","old"]],"lastEventId":"42","reconnectMax":3,"reconnectDelayMs":0}`, "",
			func(r Request) bool {
				return r.SSE != nil && r.SSE.Reconnects == 3 && r.SSE.ReconnectDelayMS != nil &&
					*r.SSE.ReconnectDelayMS == 0 && r.Headers["Last-Event-ID"] == "42"
			}},
		{"websocket", "wss://example.com/socket", `{"type":"websocket","subprotocols":["fixture"],"messages":[{"encoding":"text","data":"hello"},{"encoding":"binary","data":"AP8="}],"receiveLimit":2,"reconnectMax":1,"reconnectDelayMs":0}`, "",
			func(r Request) bool {
				return r.WebSocket != nil && r.WebSocket.Subprotocols == "fixture" && len(r.WebSocket.Messages) == 2 &&
					*r.WebSocket.ReceiveLimit == 2 && r.WebSocket.ReconnectMax == 1 && r.WebSocket.ReconnectDelay() == 0
			}},
		{"mqtt", "mqtt://localhost:1883", `{"type":"mqtt","topic":"demo/events","payload":"hello"}`, "hello",
			func(r Request) bool {
				return r.MQTT != nil && r.MQTT.Topic == "demo/events" && r.MQTT.Mode == "publish" &&
					r.MQTT.ReceiveLimit == 1 && r.MQTT.KeepAlive == 30
			}},
		{"mqtt", "mqtts://broker.example.com", `{"type":"raw","value":{"mode":"subscribe","topic":"demo/#","qos":2,"cleanSession":false,"keepAliveSeconds":0,"receiveLimit":4}}`, "",
			func(r Request) bool {
				return r.MQTT != nil && r.MQTT.PersistentSession && r.MQTT.KeepAlive == 0 &&
					r.MQTT.ReceiveLimit == 4 && r.MQTT.QoS == 2
			}},
		{"amqp", "amqps://broker.example.com/jobs", `{"type":"raw","value":{"mode":"consume","queue":"jobs","exchange":"events","exchangeType":"topic","routingKey":"demo.#","declare":false,"autoAck":true}}`, "",
			func(r Request) bool {
				return r.AMQP != nil && r.AMQP.Queue == "jobs" && r.AMQP.ExchangeType == "topic" &&
					r.AMQP.RoutingKey == "demo.#" && !r.AMQP.Declare && r.AMQP.AutoAck
			}},
		{"redis", "localhost:6379", `{"type":"raw","value":{"database":2,"commands":[["GET","{{key}}"]]}}`, `[["GET","{{key}}"]]`,
			func(r Request) bool { return r.Redis != nil && r.Redis.Protocol == 2 && r.Redis.Database == "2" }},
		{"sql", "postgres://localhost/jobs", `{"type":"raw","value":{"sql":"SELECT $1","parameters":[9007199254740993],"username":"reader","passwordRef":"local-db","rowLimit":12,"transactional":true}}`, "SELECT $1",
			func(r Request) bool {
				return r.SQL != nil && r.SQL.Parameters == "[9007199254740993]" && r.SQL.Username == "reader" &&
					r.SQL.PasswordRef == "local-db" && r.SQL.RowLimit == 12 && r.SQL.Transactional
			}},
		{"kafka", "kafka://localhost:9092", `{"type":"raw","value":{"mode":"produce","topic":"jobs","partition":0,"encoding":"base64","payload":"AP8=","passwordRef":""}}`, "AP8=",
			func(r Request) bool {
				return r.Kafka != nil && r.Kafka.Mode == "produce" && r.Kafka.Partition == "0" &&
					r.Kafka.Encoding == "base64" && r.Kafka.Topic == "jobs"
			}},
		{"kafka", "localhost:9093", `{"type":"raw","value":{"mode":"consume","topic":"{{key}}","partition":null,"groupId":"reader","offsetReset":"earliest","receiveLimit":3,"autoCommit":true,"securityProtocol":"SASL_SSL","saslMechanism":"SCRAM-SHA-512","username":"reader","passwordRef":"jobs","caPemRef":"ca","certificatePemRef":"cert","keyPemRef":"key"}}`, "",
			func(r Request) bool {
				return r.Kafka != nil && r.Kafka.Mode == "consume" && r.Kafka.Topic == "{{key}}" && r.Kafka.Partition == "" &&
					r.Kafka.GroupID == "reader" && r.Kafka.OffsetReset == "earliest" && r.Kafka.ReceiveLimit == "3" &&
					r.Kafka.AutoCommit && r.Kafka.SecurityProtocol == "SASL_SSL" && r.Kafka.SASLMechanism == "SCRAM-SHA-512" && r.Kafka.Username == "reader" && r.Kafka.PasswordRef == "jobs" &&
					r.Kafka.CAPEMRef == "ca" && r.Kafka.CertificatePEMRef == "cert" && r.Kafka.KeyPEMRef == "key"
			}},
		{"grpc", "https://grpc.example.com", `{"type":"grpc","service":"demo.Echo","method":"Watch","mode":"server_streaming","messageJson":"{\"key\":\"{{key}}\"}","metadata":[["x-trace","test"]]}`, `{"key":"{{key}}"}`,
			func(r Request) bool {
				return r.GRPC != nil && r.GRPC.UseJSON && r.GRPC.Mode == "server_streaming" && r.Headers["x-trace"] == "test"
			}},
	} {
		t.Run(tc.protocol+"/"+tc.target, func(t *testing.T) {
			input := `{"name":"Demo","protocolId":"` + tc.protocol + `","target":"` + tc.target + `","payload":` + tc.payload + `,
				"environmentRef":"default-env","authRef":null,"timeoutMs":30000,"proxy":null,
				"tls":{"verify":true,"client_cert_ref":null},"retryPolicy":{"max_retries":0,"backoff_ms":0},
				"metadata":{},"variables":{"key":"fixture"}}`
			value, err := Read(strings.NewReader(input))
			if err != nil {
				t.Fatal(err)
			}
			request := value.Requests[0]
			if !tc.check(request) || request.Body != tc.wantBody || request.Variables["key"] != "fixture" {
				t.Fatal("legacy settings were lost")
			}
			data, err := Encode(value)
			if err != nil {
				t.Fatal(err)
			}
			restored, err := Read(bytes.NewReader(data))
			if err != nil || !reflect.DeepEqual(value.Requests, restored.Requests) {
				t.Fatalf("legacy settings did not round-trip: %v", err)
			}
		})
	}
}

func TestReadLegacyKafkaRejectsCredentialsAndUnsupportedSecurity(t *testing.T) {
	for _, extra := range []string{
		`"password":"fixture"`, `"keyPemRef":"fixture"`,
		`"securityProtocol":"SASL_PLAINTEXT"`,
	} {
		input := `{"protocolId":"kafka","target":"localhost:9092","payload":{"type":"raw","value":{"topic":"jobs",` + extra + `}}}`
		if _, err := Read(strings.NewReader(input)); err == nil || strings.Contains(err.Error(), "fixture") {
			t.Fatal("expected a sanitized Kafka import error")
		}
	}
}

func TestReadLegacyStreamRejectsUnsupportedExecutionOptions(t *testing.T) {
	for _, input := range []string{
		`{"protocolId":"sse","target":"https://example.com","payload":{"type":"sse","reconnectMax":2,"reconnectDelayMs":60001}}`,
		`{"protocolId":"websocket","target":"wss://example.com","payload":{"type":"websocket","messages":[{"encoding":"binary","data":"not base64"}]}}`,
		`{"protocolId":"tcp","target":"localhost:9000","payload":{"type":"tcp","data":"ff","encoding":"hex","sendCount":10001}}`,
		`{"protocolId":"udp","target":"localhost:9000","metadata":{"socketPayloadFormat":"json"},"payload":{"type":"udp","data":"{}","encoding":"text"}}`,
	} {
		if _, err := Read(strings.NewReader(input)); err == nil {
			t.Fatalf("silently accepted unsupported stream settings: %s", input)
		}
	}
}

func TestLegacyGRPCPreservesTimeout(t *testing.T) {
	input := `{"protocolId":"grpc","target":"http://localhost:50051","timeoutMs":1250,"payload":{"type":"grpc","service":"demo.Echo","method":"Watch","mode":"server_streaming","messageBase64":"CAE="}}`
	value, err := Read(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	data, err := Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Read(bytes.NewReader(data))
	if err != nil || reopened.Requests[0].GRPC.TimeoutMS != 1250 {
		t.Fatalf("gRPC timeout did not round-trip: %v", err)
	}
}

func TestLegacyRPCTimeoutAndPayloadRoundTrip(t *testing.T) {
	for _, tc := range []struct{ protocol, payload, body string }{
		{"jsonrpc", `{"type":"raw","value":{"method":"lookup","params":[1],"id":9007199254740993,"headers":[["X-Test","yes"]]}}`, "[1]"},
		{"soap", `{"type":"raw","version":"1.1","action":"urn:lookup","envelope":"<Envelope/>","headers":[["X-Test","yes"]]}`, "<Envelope/>"},
	} {
		t.Run(tc.protocol, func(t *testing.T) {
			input := `{"protocolId":"` + tc.protocol + `","target":"http://localhost/","timeoutMs":1250,"payload":` + tc.payload + `}`
			value, err := Read(strings.NewReader(input))
			if err != nil {
				t.Fatal(err)
			}
			request := value.Requests[0]
			if request.RPC == nil || request.RPC.TimeoutMS != 1250 || request.Body != tc.body || request.Headers["X-Test"] != "yes" {
				t.Fatal("legacy RPC fields were lost")
			}
			if tc.protocol == "jsonrpc" && (request.RPC.ID != "9007199254740993" || request.RPC.Method != "lookup") {
				t.Fatal("legacy JSON-RPC settings were lost")
			}
			if tc.protocol == "soap" && (request.RPC.Version != "1.1" || request.RPC.Action != "urn:lookup") {
				t.Fatal("legacy SOAP settings were lost")
			}
			data, err := Encode(value)
			if err != nil {
				t.Fatal(err)
			}
			reopened, err := Read(bytes.NewReader(data))
			if err != nil || !reflect.DeepEqual(value.Requests, reopened.Requests) {
				t.Fatalf("RPC project did not round-trip: %v", err)
			}
		})
	}
}

func TestLegacyBrokerTimeoutRoundTrip(t *testing.T) {
	for _, tc := range []struct{ protocol, payload string }{
		{"redis", `{"type":"raw","value":{"commands":[["PING"]]}}`},
		{"mqtt", `{"type":"mqtt","topic":"jobs"}`},
		{"amqp", `{"type":"raw","value":{"mode":"publish"}}`},
		{"kafka", `{"type":"raw","value":{"topic":"jobs"}}`},
	} {
		t.Run(tc.protocol, func(t *testing.T) {
			input := `{"protocolId":"` + tc.protocol + `","target":"localhost:9092","timeoutMs":1234,"payload":` + tc.payload + `}`
			value, err := Read(strings.NewReader(input))
			if err != nil {
				t.Fatal(err)
			}
			data, err := Encode(value)
			if err != nil {
				t.Fatal(err)
			}
			restored, err := Read(bytes.NewReader(data))
			if err != nil || restored.Requests[0].BrokerTimeoutMS != 1234 {
				t.Fatalf("legacy broker timeout did not round-trip: %v", err)
			}
		})
	}
}

func TestReadLegacyRequestRejectsSettingsItCannotPreserve(t *testing.T) {
	for _, fields := range []string{
		`"timeoutMs":300001,"payload":{"type":"mqtt","topic":"jobs"}`,
		`"preScripts":["fixture"],"payload":{"type":"mqtt","topic":"jobs"}`,
		`"payload":{"type":"raw","value":{"topic":"jobs","passwordRef":" fixture"}}`,
		`"payload":{"type":"mqtt","topic":"jobs","caPemRef":"fixture"}`,
		`"payload":{"type":"mqtt","topic":"jobs","unknownField":true}`,
		`"payload":{"type":"amqp","topic":"jobs"}`,
	} {
		input := `{"protocolId":"mqtt","target":"mqtt://localhost",` + fields + `}`
		if _, err := Read(strings.NewReader(input)); err == nil {
			t.Fatalf("silently accepted unsupported legacy settings: %s", fields)
		}
	}
}
