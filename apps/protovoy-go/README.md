# ProtoVoy Go rewrite

This module is the first part of the migration to Go and Viva. It includes an
HTTP engine, a CLI, and a shared request workbench for desktop and browsers.

From this directory, with Go 1.26 or newer:

```sh
go test ./...
go run ./cmd/protovoy -timeout 10s http://127.0.0.1:8080/
```

From the repository root, `node scripts/validate-changed.mjs --go` runs the native
tests and compiles the WebAssembly workbench when relevant files change.

Build the native workbench, CLI, Local Agent and Web server from this directory:

```sh
go build -mod=readonly -trimpath -o build/native/ ./cmd/workbench ./cmd/protovoy ./cmd/agent ./cmd/server
```

CI uploads the executables and the Viva web distribution for Windows, Linux and macOS
as `protovoy-go-native-<OS>-<architecture>`. Extract the archive, then run
`./server -web ./web` on Linux or macOS, or `./server.exe -web ./web` in PowerShell,
from the extracted directory to serve the workbench on loopback. For a remote
deployment, use the HTTPS and authentication settings below.

Draft releases include the same bundle for Linux x64, Windows x64 and macOS arm64 as
`protovoy-viva-<tag>-<platform>.tar.gz`, with its checksum in
`SHA256SUMS.txt`. The macOS archive contains `ProtoVoy.app` for launching from
Finder. The release workflow currently builds the bundle without Developer ID
signing or notarization. Building it from source on macOS requires the Xcode
command-line tools:

```sh
go tool viva package --target darwin --format app --no-sign -o build/native/ProtoVoy.app
```

Run the **Viva bundles** workflow manually in GitHub Actions to test the same
builds on a branch. It runs the Go tests and uploads the bundles as Actions
artifacts, using the commit SHA in archive names for branch runs.

The Linux CI job also opens the built workbench in headless Chrome and tests
collection import, assertion editing, reload, export and CLI execution. To
run the same check from this directory with Node.js 22 or newer, build the native
executables and web assets into `build/native` and `build/native/web`, then run
`node scripts/smoke-web.mjs`. Set `CHROME_BIN` if Chrome is outside your PATH.

The CLI writes the HTTP status to standard error and streams the response body
to standard output. Use Ctrl+C to cancel, or `-follow` to enable redirects.

Generate a cURL command for a saved HTTP, GraphQL query, JSON-RPC or SOAP
request with `go run ./cmd/protovoy curl -request 1 PROJECT.json`. The number
is the request position in the collection. The CLI resolves collection and
request variables and prints the command for Bash or another POSIX shell.
In the workbench, select a saved request and choose **Export saved cURL**
to download the same command as a text file.
The generator requires redirects to be disabled and the encoded body to be
UTF-8 without NUL bytes. It rejects recognizable credentials, Agent credential
bindings, and requests with pre-request or post-response scripts.
Remove manual `Content-Length` and `Transfer-Encoding` headers so cURL can
frame the generated request body.

Import a literal cURL command into a one-request project by piping the
command text to `go run ./cmd/protovoy import-curl` and saving its JSON output.
The importer supports a single HTTP(S) URL, a method, headers, inline data,
redirects and a timeout. It rejects shell expansions, file references,
unsupported options and recognizable credentials.
Long options accept `--option=value`. With `--compressed`, ProtoVoy lets
its HTTP transport negotiate response compression.
Use `--data-urlencode` for literal form values and `-G` or `--get` to
place the data in the URL query.
Use `--json` for inline JSON data. It sets JSON Content-Type and Accept
defaults and can be repeated to join parts of the body.
In the workbench, choose **Import cURL**, paste the command, then choose
**Load request draft**. Review the result and use **Save request** to add it
to your collection.

Choose **Import project** to open a HAR 1.2 file up to 8 MiB. The importer
creates a collection from the captured HTTP requests, including inline
bodies and URL-encoded form parameters. It discards responses, cookie data,
credential headers and transport-managed headers. Remove credentials from
captured URLs and request bodies before importing.

**Import project** also accepts Postman Collection v2.1 JSON. It preserves
folders, HTTP requests, raw bodies, literal URL-encoded form fields and
collection variables. Variables referenced by the requests must be present
in the collection. Imports with active collection, folder or request scripts
return an error. Postman authorization settings and saved responses are omitted.

**Import project** also accepts OpenAPI 3.0 and 3.1 in JSON or YAML.
YAML imports accept a single document without aliases or merge directives.
It creates HTTP requests from the operations, using the first server at
the most specific level, scalar parameter examples or defaults, and inline
JSON body examples. Define `baseUrl` for relative servers and supply values
for path parameters without examples. Local JSON Pointer references are
supported for path items, parameters, request bodies, schemas and named examples. Resolve references
to external documents before importing. Without an explicit JSON example,
the importer generates an editable body from object, array and scalar schemas.
It combines top-level object fields from `allOf` and selects the first
branch it can generate from `oneOf` or `anyOf`.
It skips read-only properties and recognizable credential fields. Review
generated values against the API's constraints before sending.

Use `-data` to send an HTTP body and repeat `-header` to send multiple
headers. The method stays GET unless you set `-method`.

```sh
go run ./cmd/protovoy -method POST -header 'Content-Type: application/json' \
  -header 'X-Trace: fixture' -data '{"name":"demo"}' http://127.0.0.1:8080/
```

Use `-data-file payload.json` or `-data-file -` to read up to 8 MiB from a
file or standard input. The CLI preserves the bytes, including trailing
newlines, and applies the selected protocol's usual payload encoding.
`-data-file` and `-data` are mutually exclusive.

Use `go run ./cmd/protovoy run PROJECT.json` to run an exported Viva
collection in file order. The runner supports HTTP, GraphQL over HTTP,
JSON-RPC, SOAP, SQL, Redis, gRPC, TCP, UDP, MQTT, AMQP, Kafka, WebSocket, SSE and
GraphQL subscriptions. It applies
collection and request variables, with
request variables taking precedence. Each request emits a JSON result to
stdout. The command exits with status 1 if any request fails to complete or
returns an unexpected HTTP status, Redis command error, nonzero gRPC status, or
an AMQP publication that the broker rejects or returns. Use
`run -fail-fast PROJECT.json` to stop at
the first failure. GraphQL results with an error list containing entries fail
even when the HTTP status is 2xx or a subscription also returns partial data.
JSON-RPC responses with an error also fail even when HTTP returns 2xx.
SOAP Faults also fail regardless of a successful HTTP status. The runner
checks GraphQL, JSON-RPC and SOAP HTTP response bodies up to 8 MiB. An empty
successful SOAP response is accepted for one-way operations.
In a v2 project file, set `expectedHttpStatus` on an HTTP, GraphQL HTTP,
JSON-RPC or SOAP request to require a specific HTTP status from 200 to 599.
For example, `"expectedHttpStatus": 404` makes HTTP 404 the expected result.
With no expected status configured, HTTP 2xx is the default.
Protocol errors in 2xx responses still fail. Set **Expected HTTP status
(collection runs)** in the workbench and save the request before exporting.
Collections with unsupported protocols are rejected before execution.
Set `responseBodyContains` in the same v2 request to check for literal,
case-sensitive text in the response body. The configured text can use up
to 4 KiB. Use `{{name}}` to resolve an expected value from the same variable
scopes as the request. The resolved value must contain 1 to 4096 bytes.
The runner searches the response as it arrives. Successful
GraphQL, JSON-RPC and SOAP responses still have an 8 MiB limit for protocol
validation. The check also works
with an expected non-2xx status. In the workbench, enter the text in
**Response body contains (collection runs)** and save the request.

Use `run -data CASES.json PROJECT.json` to run the collection once per
data row. JSON input is an array of objects, and CSV input uses its first
row as variable names. Rows run in file order. Collection variables override
row values, and request variables override both. Each JSON result includes
a one-based `requestIndex`, elapsed execution time in `durationMs`, and,
when a data file is supplied, a one-based `iteration`. With `-fail-fast`, the
first failure also stops later iterations.

Add `-concurrency N` to run up to 16 data rows concurrently. Requests
within a row still run in project order, and reports are emitted in data-file
order. Use sequential execution when you need `-fail-fast`.

Add `-junit RESULT.xml` to write a JUnit report alongside the JSON output.
If `-fail-fast` stops the run early, the report includes one skipped summary
with the number of requests that did not run.
An interrupted run adds a collection execution error to the report.
The CLI handles Ctrl+C and SIGTERM through the collection cancellation path.
The report includes completed requests and their failure details, including
when the run exits with a failed request. Use a new output path for each run.

SSE succeeds when the stream ends cleanly and configured reconnects finish.
GraphQL subscriptions succeed when the operation completes. Set
`-stream-timeout` to change the per-request deadline from 30 seconds.
Reaching that deadline marks the request as failed. WebSocket requests replay
the saved startup messages on each connection and count messages across
reconnects. A configured receive limit must be reached for success. Without
a receive limit, a normal close succeeds after configured reconnects.

Collection execution reports outcomes without including database rows or Redis
reply bodies. SQL and Kafka can use named credentials bound to configured
endpoints.
The runner does not attach the single-request CLI's environment passwords to
arbitrary targets in project files.

Use `-protocol sse` to read a server-sent event stream. The CLI writes
one JSON line per event with `type`, `id`, and `data`. Use
`-sse-reconnects` to resume after a clean disconnect and
`-sse-reconnect-delay-ms` to set the initial retry delay.
`-timeout` bounds the entire command, including reconnects.

```sh
go run ./cmd/protovoy -protocol sse -sse-reconnects 2 -timeout 2m http://127.0.0.1:8080/events
```

Use `-protocol websocket` to connect to a WebSocket endpoint. Supply
`-data` to send one initial text message, or add `-encoding base64` to send
binary bytes. Received messages are emitted as JSON lines with `encoding`
and `data`. Binary data uses Base64. A positive `-ws-receive-limit` stops
after that many messages, and `-timeout` bounds the connection.

```sh
go run ./cmd/protovoy -protocol websocket -ws-subprotocols echo \
  -ws-receive-limit 1 -data hello ws://127.0.0.1:8080/socket
```

For GraphQL, pass the document with `-data`, optional JSON variables with
`-graphql-variables`, and an operation name with `-graphql-operation`.
The same `-header` flag applies to GraphQL HTTP requests and subscription
handshakes.

```sh
go run ./cmd/protovoy -protocol graphql -data 'query { __typename }' http://127.0.0.1:8080/graphql
go run ./cmd/protovoy -protocol graphql -graphql-subscribe -timeout 2m \
  -data 'subscription { updates }' ws://127.0.0.1:8080/graphql
```

Queries and mutations use HTTP POST. Subscriptions use
`graphql-transport-ws` and emit one JSON payload per line until completion,
cancellation, or the execution timeout.

For JSON-RPC 2.0, pass the method with `-rpc-method` and JSON parameters with
`-data`. The default ID is 1. Use `-rpc-id` to send a specific JSON ID.
For SOAP, pass the XML envelope with `-data` and optionally set
`-soap-version` and `-soap-action`. Both commands accept repeated headers.

```sh
go run ./cmd/protovoy -protocol jsonrpc -rpc-method ping -data '{}' http://127.0.0.1:8080/rpc
go run ./cmd/protovoy -protocol soap -soap-version 1.1 -soap-action '"urn:ping"' \
  -data '<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body/></soap:Envelope>' \
  http://127.0.0.1:8080/soap
```

Use `-protocol sql` with a PostgreSQL, MySQL or SQLite URL. Pass the
statement with `-data` and bound arguments as a JSON array with
`-sql-parameters`. The CLI emits one JSON result containing rows or affected
row counts. Use `-sql-transaction` for transactional execution and
`-sql-row-limit` to cap returned rows. For PostgreSQL or MySQL, pass the
username with `-sql-username` and supply the password through
`PROTOVOY_SQL_PASSWORD`.

```sh
go run ./cmd/protovoy -protocol sql -data 'SELECT ? AS id' -sql-parameters '[7]' sqlite::memory:
```

Use `-protocol mqtt` to publish or subscribe with MQTT 3.1.1. Set the topic
or filter with `-mqtt-topic` and choose QoS with `-mqtt-qos`. Publish
accepts text or Base64 data and the `-mqtt-retain` flag. Subscriptions
emit one JSON message per line and stop at `-mqtt-receive-limit` or
`-timeout`. Use `mqtts://` for TLS and optionally provide a CA file
with `-ca`. Supply an MQTT password through `PROTOVOY_MQTT_PASSWORD`
with `-mqtt-username`.

Saved MQTT settings can use `passwordRef` instead of a password. In Viva,
open **Connection settings** and enter **MQTT password reference**. The CLI
accepts `-mqtt-password-ref REFERENCE`; saved collection requests use the same
binding. Configure `PROTOVOY_MQTT_PASSWORD_REF`, `PROTOVOY_MQTT_TARGET` (an exact
`mqtts://` URL), and `PROTOVOY_MQTT_USERNAME` in the execution host. Optionally
pin `PROTOVOY_MQTT_SERVER_NAME` and `PROTOVOY_MQTT_CA_FILE`. Reference execution
rejects a different broker, username, TLS name, inline password or inline CA
before reading the secret. Reference names are literal and are not interpolated.
By default the bound value comes from `PROTOVOY_MQTT_PASSWORD`. For desktop and
CLI, set `PROTOVOY_MQTT_PASSWORD_KEYCHAIN=1` to resolve it from the system
credential store on each execution; save the value with `protovoy secret set
REFERENCE`. Start the Local Agent with `-mqtt-password-keychain` for the same
explicit opt-in. Projects and workspace snapshots retain only the reference
name; the Agent resolves it locally and session passwords are not saved.
`-mqtt-password-ref` cannot be combined with `-password-secret`.

Hosted MQTT still uses the server's configured credentials. Set
`PROTOVOY_MQTT_PASSWORD_REF` on the server to name that binding, alongside its
TLS endpoint, username and `PROTOVOY_MQTT_PASSWORD`. The browser sends the saved
reference as `expectedPasswordRef`, and the server rejects a mismatch before
connecting. The hosted bridge does not accept browser-supplied password values.

```sh
go run ./cmd/protovoy -protocol mqtt -mqtt-topic demo -mqtt-qos 1 -data hello mqtt://127.0.0.1:1883
go run ./cmd/protovoy -protocol mqtt -mqtt-subscribe -mqtt-topic 'demo/#' \
  -mqtt-receive-limit 3 mqtt://127.0.0.1:1883
```

Use `-protocol amqp` to publish with broker confirms or consume with
`-amqp-consume`. By default the command uses existing broker topology.
Use `-amqp-declare` to declare the configured exchange and, when consuming,
the queue and its binding. Consumed messages are acknowledged after their
JSON result is written. Publish results include the broker confirmation and
a `published` boolean. Use `amqps://` for TLS and supply a password through
`PROTOVOY_AMQP_PASSWORD` with `-amqp-username`.

AMQP projects can instead store `passwordRef`. In Viva **Request options**,
enter **AMQP password reference**; the CLI also accepts `-amqp-password-ref` and
uses saved collection references. Configure `PROTOVOY_AMQP_PASSWORD_REF`,
`PROTOVOY_AMQP_TARGET` (the exact `amqps://` URI, including its virtual host),
and `PROTOVOY_AMQP_USERNAME` on the execution host. Optional
`PROTOVOY_AMQP_SERVER_NAME` and `PROTOVOY_AMQP_CA_FILE` pin TLS identity and
trust. A different endpoint, virtual host, username, TLS name, inline password
or inline CA is rejected before secret lookup. Reference names are literal.
The bound password comes from `PROTOVOY_AMQP_PASSWORD` by default. Set
`PROTOVOY_AMQP_PASSWORD_KEYCHAIN=1` for desktop/CLI system-credential lookup, or
start the Local Agent with `-amqp-password-keychain`; save the named value with
`protovoy secret set REFERENCE`. Credential errors do not include secret-store
details. Projects and workspaces preserve the reference without saving session
passwords. `-amqp-password-ref` conflicts with `-password-secret`.
For hosted execution, set `PROTOVOY_AMQP_PASSWORD_REF` to name the server's
configured TLS endpoint and credentials. The browser sends `expectedPasswordRef`
and mismatches are rejected before connection; browser-supplied passwords are
not accepted by that bridge.

```sh
go run ./cmd/protovoy -protocol amqp -amqp-routing-key jobs -data hello amqp://127.0.0.1:5672/
go run ./cmd/protovoy -protocol amqp -amqp-consume -amqp-queue jobs \
  -amqp-receive-limit 2 amqp://127.0.0.1:5672/
```

Use `-protocol kafka` with a comma-separated list of brokers. Set
`-kafka-topic` and use `-data` for a record value. Produce accepts an
optional key and partition. Consume emits one JSON line per record and
commits its offset after writing it. Set the consumer group, offset reset,
and receive limit with `-kafka-group`, `-kafka-offset-reset`, and
`-kafka-receive-limit`. Use `-kafka-security SSL` or `SASL_SSL` for
verified TLS and supply a SASL password through `PROTOVOY_KAFKA_PASSWORD`.

```sh
go run ./cmd/protovoy -protocol kafka -kafka-topic jobs -kafka-key job-1 -data hello 127.0.0.1:9092
go run ./cmd/protovoy -protocol kafka -kafka-consume -kafka-topic jobs \
  -kafka-group demo -kafka-offset-reset earliest 127.0.0.1:9092
```

For a raw gRPC call, give the service, method and Base64 protobuf message.

```sh
go run ./cmd/protovoy -protocol grpc -grpc-service demo.Echo \
  -grpc-method Watch -grpc-mode server_streaming -data CAE= http://127.0.0.1:50051
```

The CLI emits one JSON line per received message. HTTP and HTTPS endpoints
use HTTP/2. Unary and server-streaming calls send one message. For
client-streaming and bidirectional calls, supply one Base64 message per line.
Those request messages are supplied before the call starts. The CLI reports
nonzero gRPC status codes even when the transport returns HTTP 200.
The Agent accepts the same raw gRPC payload through `/v1/executions`.
For descriptor-based JSON, use a binary `FileDescriptorSet` produced by
`protoc --include_imports --descriptor_set_out=service.pb service.proto`.
The CLI accepts `-grpc-descriptor service.pb -grpc-json -data '{"value":1}'`.
For client-streaming and bidirectional calls, pass a JSON array of messages.
Use the Agent's `descriptorSetBase64` and `messageJson` payload fields for
the same operation through `/v1/executions`. The web execution bridge accepts
these fields for its configured endpoint. Response events retain protobuf
bytes and include a JSON preview when the decoded result fits within 2 KiB.
In Viva, select **gRPC** to enter the endpoint, service, method, mode, metadata
and Base64 message. Desktop can connect directly. In a browser, use the
server's configured gRPC endpoint or pair with Local Agent and choose
**Send gRPC via Local Agent**. To enable server execution, start
`./cmd/server` with `-grpc-target http://127.0.0.1:50051`. A supplied endpoint
must match that setting exactly. An unsaved request can leave it blank.
The bridge uses the same authentication and origin checks as HTTP execution.
The panel displays
responses as they arrive and supports cancellation. **Save request** keeps
the gRPC request in the local collection and version 2 project exports.
Set **Timeout (ms)** from 1 to 300,000, or leave it blank for 30 seconds.
One deadline covers reflection and the entire call, including all streamed
messages. Legacy gRPC requests preserve custom timeouts on import.
For a JSON request in Viva, select **Use JSON**, then **Import descriptor**
and choose a binary descriptor set up to 512 KiB. Enter the message in the
JSON editor and send it using the same desktop, hosted or Agent transport.
Saving the request keeps the selected format and descriptor in the local
workspace and version 2 project exports. **Use Base64** switches the editor
back to raw protobuf, and a loaded descriptor can still decode responses.
If a JSON request has no descriptor, the CLI, Agent and Web execution
bridge use gRPC server reflection to resolve the named service and method.
The client tries v1, then v1alpha for older servers, using the same endpoint,
metadata and time budget as the call. Raw Base64 requests and requests with
an imported descriptor skip reflection.

The Go Local Agent also executes JSON-RPC 2.0 calls and SOAP 1.1/1.2
requests through the v1 `/v1/executions` endpoint. Use protocol IDs
`jsonrpc` or `soap` and the existing `raw` envelope. Responses use the
HTTP event stream and history keeps the original protocol ID. JSON-RPC
variable values are encoded as JSON, and numeric IDs retain their precision.
The Agent does not follow redirects for these requests. In Viva, choose
**JSON-RPC** or **SOAP** to edit and send these requests. Both can be saved
in collections and v2 project exports. Browser execution uses the
server-configured HTTP endpoint or a paired Local Agent. Set **Timeout (ms)**
to override the 30-second default with a value from 1 to 300,000. Legacy
JSON-RPC and SOAP envelopes retain the timeout and request fields on import.

For a Redis command, use `-protocol redis -data '["PING"]' host:6379`.
For a pipeline on one connection, use
`-protocol redis -redis-pipeline -data '[["PING"],["GET","key"]]' host:6379`.
The pipeline returns a JSON array in command order, including Redis error
replies. Each command and pipeline uses typed JSON responses, with binary bulk
strings encoded as Base64. The full request and response are each limited to
1 MiB. Set the password through the `PROTOVOY_REDIS_PASSWORD` environment
variable and optionally pass `-redis-username` for Redis ACL authentication.
Use `-redis-db` to select a database before running commands. Add `-tls`
for verified TLS and optionally `-server-name` and `-ca path/to/ca.pem`
to set the certificate name and a private CA. AUTH and SELECT must succeed
before the pipeline is sent. RESP2 is the default. Pass `-redis-resp 3` to
negotiate RESP3 with `HELLO 3` before executing any user commands.

Redis requests can store a `passwordRef` instead of a password. In Viva **Redis
connection settings**, enter **Redis password reference** and enable verified
TLS. CLI requests accept `-redis-password-ref REFERENCE` with `-tls`; collection
runs use the saved reference. Configure `PROTOVOY_REDIS_PASSWORD_REF` and
`PROTOVOY_REDIS_TARGET` (an exact `host:port`) on the execution host. Optional
`PROTOVOY_REDIS_USERNAME`, `PROTOVOY_REDIS_DATABASE`,
`PROTOVOY_REDIS_SERVER_NAME` and `PROTOVOY_REDIS_CA_FILE` bind ACL identity,
initial database and TLS trust. An omitted username uses the configured user
(or password-only AUTH), and an omitted database uses the configured number,
defaulting to 0. Explicit database assertions are compared numerically.
Different connection settings, plaintext mode, inline passwords and inline
CAs are rejected before secret lookup. These checks bind initial connection
settings; command permissions remain those of the Redis account's ACL.
The bound password defaults to `PROTOVOY_REDIS_PASSWORD`. Desktop and CLI can
set `PROTOVOY_REDIS_PASSWORD_KEYCHAIN=1` to read the named system credential on
each execution; start the Agent with `-redis-password-keychain` for that opt-in.
Use `protovoy secret set REFERENCE` to save the value. Lookup failures do not
expose secret-store error details. References cannot be combined with
`-password-secret`, and projects/workspaces retain references without passwords.
Hosted Redis uses its configured credentials and `PROTOVOY_REDIS_PASSWORD_REF`
as the binding name. Saved requests assert the reference, and any explicit
username, initial database and TLS name, before the bridge connects. Assertions
without a named binding are rejected. The hosted bridge does not accept
browser-supplied password values.

In the Viva workbench, select **Redis** and enter a JSON array of commands,
such as `[["PING"],["GET","key"]]`. Desktop connects to the host and port
you enter. To enable the browser panel, start the preview server with
`-redis-target host:6379`. The bridge only connects to that configured Redis
endpoint and checks the browser origin and preview header. Leave the target
field blank in the browser to use the configured endpoint. **Save request**
adds the Redis request to the local collection. The workspace restores its
commands, endpoint, RESP version, and non-password connection settings.
Variables in the endpoint and individual command arguments are resolved at
send time. Keep secrets out of saved commands. The password field and
responses stay in memory, and autosave pauses for recognized credential
commands such as AUTH.

Use the protocol switch in the Redis panel to choose RESP3. RESP3 responses
retain typed map keys, sets, attributes, binary data and exact textual
representations of doubles and big numbers. The bounded pipeline skips
out-of-band push frames when matching command replies. Persistent Redis
Pub/Sub sessions are not supported by this panel.

On desktop, **Redis connection settings** exposes the username, masked
password, database and verified TLS settings. For the browser, set
`PROTOVOY_REDIS_PASSWORD` in the Go server process and configure
`-redis-username`, `-redis-db`, `-redis-tls`, `-redis-server-name` and
`-redis-ca` there. The browser uses the configured Redis user's command
permissions.

The CLI also supports one TCP or UDP exchange, for example
`go run ./cmd/protovoy -protocol udp -data "00 01 ff" -encoding hex 127.0.0.1:9000`.
Socket exchanges return the first received chunk or datagram by default.
For TCP, use `-framing fixed -length 8` to read exactly eight bytes, or
`-framing delimiter -delimiter END` to read through a delimiter. Framed
responses are bounded to 512 KiB. Add `-tls` for verified TCP TLS,
`-server-name example.test` for SNI/certificate matching, and optionally
`-ca local-ca.pem` to add a private CA. Use `-count 3 -interval 100ms`
to send three times on the same connection before receiving a response.
The Viva workbench includes TCP and UDP exchange panels. Enter host:port,
text/base64/hex encoding, and the payload. Responses appear as Base64.
Set **Timeout (ms)** to override the 30-second TCP or UDP deadline. The
1–300,000 ms limit covers connection, send intervals, and response reads.
Custom timeouts are preserved when importing legacy TCP and UDP requests.
Set **Send count** and **Interval in ms** to repeat a payload on the same
connection before reading the response. The hosted bridge limits the
aggregate encoded input to 8 MiB for repeated sends.
The TCP panel supports response framing, verified TLS, a custom server name,
and optional private CA PEM certificates. These settings are retained with
the local request and draft. Private-key input pauses workspace autosave.

The preview server can enable `POST /api/socket` with `-tcp-target host:port`
and/or `-udp-target host:port`. Requests use the same origin and preview
header checks as HTTP execution. The server chooses the configured endpoint
and returns binary-safe `dataBase64` JSON. Request fields include `network`,
`expectedTarget`, `data`, `encoding`, `framing`, `delimiter`, `fixedLength`,
`tls`, and `serverName`. Execution uses a 30-second timeout.
The optional `caPem` field adds private trust certificates for TCP TLS.
It accepts certificate-only PEM up to 1 MiB and rejects private-key blocks.

Run the desktop workbench with `go run ./cmd/workbench`. Enter an HTTP or HTTPS
URL and select **Send** to make a request from the machine running ProtoVoy.
The method defaults to GET. Enter headers one per line as `Name: value` and
enter a text body when needed. The workbench shows the HTTP status and streams
up to 8 KiB of the response into the preview. Select **Cancel** to stop reading.

Use **Save request** to add or update a request, then **Export project** to
download the collection. HTTP-only collections without collection variables
use the legacy `protovoy-project` version 1 format. Collections with protocol
settings or collection variables use `protovoy-multiprotocol-project` version
2. **Import project** opens either format. Version 2 carries GraphQL
operations and subscriptions, SSE, WebSocket, TCP, UDP, Redis, and gRPC settings,
along with request and collection variables. The distinct format identifier
keeps old clients from interpreting these requests as HTTP.

Both formats preserve folder paths and store one value per header name.
The importer also opens standalone legacy HTTP, gRPC, Redis, MQTT, AMQP,
Kafka, SQL, JSON-RPC, SOAP, TCP, UDP, SSE and interactive WebSocket
request envelopes, including `examples/mqtt/publish.local.json`. These
requests can be saved in the workspace and exported as v2 projects.
The importer reports settings that cannot be preserved, including
unsupported secret references and nondefault timeouts for protocols other
than HTTP, GraphQL over HTTP, JSON-RPC, SOAP, gRPC, SQL, TCP, UDP, Redis,
MQTT, AMQP, and Kafka.
V1 team snapshots can import these requests and GraphQL into one collection,
with workspace, project, module and nested collection names kept in each
request's folder path. A missing request or unsupported setting rejects the
snapshot as a whole. Empty collection folders appear in the workbench and
survive v2 project export and workspace restore.
Imported HTTP redirect settings are saved in v2. Desktop and paired Agent
requests can follow redirects. For the hosted execution bridge, turn
**Follow redirects** off to use the configured endpoint.
Export checks for possible credentials in headers, URLs, request data, and
variable names. Global and environment variables stay in the local workspace.

The current collection, selected request, and draft are saved locally after a
short delay and restored when you reopen the workbench. Desktop stores the
snapshot in `protovoy-go-workspace.json` under `PROTOVOY_CONFIG_DIR` or the
per-user `protovoy` config directory. Browser storage is scoped to the current
site and browser profile. Autosave pauses when it detects possible credentials
or another window has changed the saved snapshot. Export a portable project
file to transfer your work to a different profile or installation.

Open **Variables** to edit global and collection values or manage named
environments. Use one `name=value` per line, save the values, and select
**Use environment** to activate an environment. When sending a request,
the more specific scope wins in this order: global, active environment,
collection, then request. The reserved variables `{{$uuid}}`,
`{{$timestamp}}`, and `{{$isoTimestamp}}` generate values at send time.
The local workspace retains these settings. Version 1 exports carry request
variables. Version 2 exports also carry collection variables.

To migrate old Local Agent variables, save the JSON response from
`GET /v1/environments` or `GET /v1/environments/default`. Open **Variables**
and choose **Import legacy environments**, then select **Use environment**
to activate the imported values. Duplicate names and possible credentials
cause the import to fail without changing the workspace. Imported header,
query and body defaults apply to HTTP requests while that environment is
active. Explicit request values take precedence. Query defaults require a URL
and the hosted bridge still requires the final URL to match its configured
endpoint. The import reports cookie defaults and keychain references that
require separate migration. Legacy project IDs and request environment
bindings are outside this file import.

Use **Switch to GraphQL** to send queries or mutations over HTTP. Enter the
document, a JSON object for GraphQL variables, and an optional operation name.
Set **Timeout (ms)** to override the 30-second HTTP deadline. Values from
1 to 300,000 are preserved in standalone and v2 project files. WebSocket
subscriptions use their existing connection lifecycle.
The editor resolves workspace templates in GraphQL string values and keys
before sending a JSON POST. Choose **Use subscription** and **Subscribe**
to use graphql-transport-ws through the WebSocket connection. The client
waits for acknowledgement, handles protocol ping/pong, and sends Complete
when you select **Stop subscription**. Browser subscriptions use the
server's configured `-websocket-target`.
You can keep GraphQL requests in the local collection and use **Export
GraphQL** for standalone request JSON. **Import project** also opens this
standalone format, including `examples/graphql/typename.query.json`.
Legacy GraphQL requests use the same import checks as other protocols.
Import rejects execution settings that cannot be carried over, such as
custom TLS verification or retry policies.

Use **Switch to SSE** to open a server-sent events stream. The workbench
shows the latest 20 events as they arrive and supports **Disconnect**.
Set up to 10 reconnect attempts to resume after the server closes a stream.
Set the initial delay to 0–60000 ms, or leave it blank for 1000 ms.
Reconnects send the last event ID. A server-sent retry interval overrides
the initial delay, up to 60 seconds. To start at a particular ID, enter a `Last-Event-ID`
header. SSE requests can be saved in the local collection.

Use **WebSocket** to open a `ws://` or `wss://` connection. Optional
subprotocols are comma-separated. After connecting, use **Send text** or
enter Base64 and use **Send binary**. The workbench shows the most recent
sent and received messages and supports **Disconnect**. Connections use
the same configured route for each retry. **Connection options** accepts
JSON with `messages` (text or Base64 binary), `receiveLimit`,
`reconnectMax`, `reconnectDelayMs` and `idleTimeoutMs`. Startup messages
send in order after each connection. The receive limit counts incoming
messages across retries. These options are kept when importing a legacy
request, saving the workspace or exporting a v2 project. An omitted idle
timeout leaves new interactive sessions open until closed or cancelled.
The workbench uses the [coder/websocket](https://github.com/coder/websocket) library.

To run the browser workbench:

```sh
go tool viva build --target web -o build/web
go run ./cmd/server -target http://127.0.0.1:8080/
```

To enable WebSocket in the browser, add
`-websocket-target ws://127.0.0.1:8080/socket`. The bridge only connects to
that configured endpoint. Desktop connects directly to the URL you enter.

Open `http://127.0.0.1:39300/` and select **Send**. The Go server serves the
WASM build and uses the endpoint configured with `-target` by default.
Repeat `-allow-http-target` to allow additional exact HTTP or HTTPS URLs.
The request URL must match an allowed URL, including its path and query.
In a short window, scroll the workbench to reach the request actions and
response. Long responses can also be scrolled inside their own panel.
On touchscreens, swipe vertically on the workbench to scroll.
In narrow windows the collection moves above the request, and the HTTP,
GraphQL, SSE and WebSocket controls wrap to fit the editor.
The browser can send GET, HEAD, POST, PUT, PATCH, DELETE, and OPTIONS requests.
Select **Response headers** to inspect the upstream HTTP header preview.
The preview retains repeated header values and masks credential and cookie headers.
For finite HTTP responses, **Headers** and **Preview** show the elapsed time
until the workbench receives response headers and finishes reading the preview.
The displayed byte count is the preview size before text decoding.
For a complete JSON object or array, use **Format JSON** to indent the
preview and **Show raw JSON** to return to the received text.
**Save preview bytes** exports the received preview before UTF-8 decoding
or JSON formatting, up to the same 8 KiB preview limit.
The server caps the JSON request payload at 1 MiB and returns the first 8 KiB
of the response for preview. Redirects are displayed without following them.
For ordinary HTTP requests, the workbench accepts a timeout from 1 to
300,000 ms. Leave the field blank for the 30-second default. Desktop,
Local Agent, and hosted execution apply it to the request. Legacy HTTP
timeout values in this range survive import and v2 export.
The Go HTTP execution APIs accept `bodyEncoding: "base64"` for binary bodies
or a `multipart` array of named parts. Each part has a `value` and optional
`fileName`, `contentType`, and `base64`. Omit the request's `Content-Type`
header for multipart requests so the execution host can generate the boundary.
In a v2 project, use multipart parts without a separate `body` or
`bodyEncoding: "base64"`.
Project export and workspace autosave check decoded Base64 bodies and
multipart parts for recognizable credentials.
Multipart requests support up to 128 parts and a 1 MiB encoded body. The
filename is transmitted as metadata. The execution host uses the supplied
part bytes and does not read from that path. Select **Base64** or
**Multipart** under **Body format** in the HTTP workbench. Multipart uses
a JSON array in the editor. Legacy inline Base64 and multipart requests
can be imported and saved or exported as v2 projects. Imported legacy
GraphQL, JSON-RPC and MessagePack HTTP requests retain their original JSON
body source. Open **Edit body source** and choose **Apply source changes**
to regenerate the request body. Sending uses the stored wire body until
source changes are applied. Editing the wire body directly removes the
source on save. Other imported JSON sources can be viewed read-only.
To create a MessagePack request, select **MessagePack** under **Body format**,
enter JSON, and apply the source. An empty method becomes POST and an
absent Content-Type becomes `application/msgpack`. The request keeps both
the JSON source and its encoded body when saved.
Browser imports read the file locally. When a request comes from an imported
project, its resolved URL must match a configured endpoint.
The server listens on loopback by default. To host the Viva workbench over
HTTPS, set `PROTOVOY_WEB_USER` in the server's environment. Supply a password
of at least 24 printable ASCII characters through `PROTOVOY_WEB_PASSWORD`
or a single-line file passed to `-web-password-file`.
Supply a certificate chain and its private key outside the web build directory.

```sh
go tool viva build --target web -o build/web
go build -o protovoy-web ./cmd/server
./protovoy-web -listen 0.0.0.0 -port 8443 \
  -public-origin https://workbench.example.com:8443 \
  -tls-cert /etc/protovoy/fullchain.pem -tls-key /etc/protovoy/privkey.pem \
  -target http://127.0.0.1:8080/
```

The browser prompts for the configured account. HTTPS and authentication
cover the workbench files and every execution API. The configured public
origin is checked on execution requests and WebSocket upgrades. Hosted mode
uses one operator account and the same server-configured targets.

For a browser deployment that uses paired Local Agents for all requests,
start the server without any execution target flags. The server serves the
Viva assets, and execution routes without a configured target return
HTTP 503. HTTPS and authentication are still required for remote access.
Configure the exact hosted HTTPS origin on each Local Agent.

### Docker Compose deployment

`GET /healthz` returns HTTP 204 when the Web server can read the workbench HTML,
JavaScript loader, and WebAssembly module. It returns HTTP 503 if those assets
become unavailable. Hosted deployments require the configured Web credentials.
Hosted probes use the configured HTTPS origin and Basic authentication.
Compose gives the Web container 12 seconds to stop. The Go server cancels
active SSE, WebSocket, gRPC and message broker executions during shutdown
and allows up to eight seconds for HTTP requests to drain.

The standalone Viva Compose service builds the Go server and WebAssembly
assets into one image. The Go server checks that the HTML, WebAssembly loader
and module are all present and readable before it starts. From the repository
root, set these variables in the
environment or an untracked Compose env file:

| Variable | Value |
| --- | --- |
| `PROTOVOY_WEB_USER`, `PROTOVOY_VIVA_PASSWORD_FILE` | Hosted account and absolute host path to a file containing a password of at least 24 printable characters. Grant container UID 65532 read access |
| `PROTOVOY_VIVA_ORIGIN` | Exact HTTPS origin, including the published port when it differs from 443 |
| `PROTOVOY_VIVA_TLS_CERT`, `PROTOVOY_VIVA_TLS_KEY` | Absolute host paths to the certificate chain and key. Grant the container's UID 65532 read access to the files |
| `PROTOVOY_VIVA_BIND`, `PROTOVOY_VIVA_PORT` | Host IP and port to publish, defaulting to `127.0.0.1:8443`. Set an explicit host IP for remote access |
| `PROTOVOY_VERSION` | Optional build version reported by the container's `protovoy-web -version` command. Defaults to `0.1.0-dev` |
| `PROTOVOY_VIVA_HTTP_TARGET`, `PROTOVOY_VIVA_WEBSOCKET_TARGET` | Optional upstream endpoints reachable from the container. Leave empty for a paired-Agent deployment |
| `PROTOVOY_VIVA_TCP_TARGET`, `PROTOVOY_VIVA_UDP_TARGET`, `PROTOVOY_VIVA_GRPC_TARGET` | Optional fixed upstreams. Use `host:port` for TCP and UDP and an HTTP/2 endpoint for gRPC |
| `PROTOVOY_VIVA_REDIS_TARGET`, `PROTOVOY_VIVA_REDIS_USERNAME`, `PROTOVOY_VIVA_REDIS_DB` | Optional fixed Redis endpoint as `host:port`, ACL username, and initial database |
| `PROTOVOY_VIVA_REDIS_TLS`, `PROTOVOY_VIVA_REDIS_SERVER_NAME` | Redis TLS defaults to `true` and uses the container's trusted roots. Set the optional certificate hostname when it differs from the target hostname |
| `PROTOVOY_REDIS_PASSWORD`, `PROTOVOY_REDIS_PASSWORD_REF` | Optional Redis password and reference name. Keep the password in the untracked deployment environment. Named references require TLS |
| `PROTOVOY_VIVA_MQTT_TARGET`, `PROTOVOY_VIVA_AMQP_TARGET` | Optional fixed broker URLs. Use `mqtts://` or `amqps://` with named passwords. Include the AMQP virtual host in its URL |
| `PROTOVOY_VIVA_MQTT_USERNAME`, `PROTOVOY_VIVA_AMQP_USERNAME` | Optional broker usernames |
| `PROTOVOY_VIVA_MQTT_SERVER_NAME`, `PROTOVOY_VIVA_AMQP_SERVER_NAME` | Optional TLS certificate hostnames. TLS uses the container's trusted roots |
| `PROTOVOY_MQTT_PASSWORD`, `PROTOVOY_MQTT_PASSWORD_REF`, `PROTOVOY_AMQP_PASSWORD`, `PROTOVOY_AMQP_PASSWORD_REF` | Optional broker passwords and reference names. Keep passwords in the untracked deployment environment |
| `PROTOVOY_VIVA_KAFKA_TARGET`, `PROTOVOY_VIVA_KAFKA_SECURITY` | Optional comma-separated Kafka broker addresses. TLS defaults to `SSL`. Use `SASL_SSL` for passwords |
| `PROTOVOY_VIVA_KAFKA_SASL`, `PROTOVOY_VIVA_KAFKA_USERNAME`, `PROTOVOY_KAFKA_PASSWORD`, `PROTOVOY_KAFKA_PASSWORD_REF` | SASL mechanism, username, password and optional reference. Keep passwords in the untracked deployment environment |
| `PROTOVOY_VIVA_SQL_TARGET`, `PROTOVOY_VIVA_SQL_USERNAME`, `PROTOVOY_SQL_PASSWORD`, `PROTOVOY_SQL_PASSWORD_REF` | Optional SQL endpoint and separate credentials. Named passwords require verified TLS. Put any file-backed SQLite database under `/var/lib/protovoy` |

For Redis with a private CA, set `PROTOVOY_VIVA_REDIS_CA_HOST_FILE` to an
absolute path to a PEM CA file readable by container UID 65532. Add
`-f deploy/viva.redis-ca.compose.yaml` after the base Compose file in the
commands below. The mount applies only to Redis TLS.
| `PROTOVOY_HTTP_ALLOWED_TARGETS` | Optional additional exact HTTP URLs, one per line. Requires `PROTOVOY_VIVA_HTTP_TARGET` |

```sh
cp deploy/viva.env.example deploy/.env.viva
# Fill in the absolute password file and certificate paths before continuing.
docker compose --env-file deploy/.env.viva -f deploy/viva.compose.yaml config --quiet
docker compose --env-file deploy/.env.viva -f deploy/viva.compose.yaml up --build -d
```

Open the configured HTTPS origin and sign in. The certificate must be trusted
by the browser and cover the origin's hostname. To verify readiness from the host,
use the configured origin and user. Curl prompts for the password.

To enable team collaboration, set `PROTOVOY_VIVA_TEAM_DB=/var/lib/protovoy/team.db`
and configure `PROTOVOY_TEAM_BOOTSTRAP_TOKEN` in the untracked env file.
The `team-data` volume holds the database across container replacements.
The container filesystem is read-only. Put the team database and its SQLite
sidecar files in `/var/lib/protovoy`, the writable data volume.
Preserve this volume when taking the deployment down. After initialization,
remove the bootstrap token and recreate the container. For OIDC, set the
`PROTOVOY_TEAM_OIDC_*` variables in the same env file.

For recurring requests, set
`PROTOVOY_VIVA_SCHEDULE_DB=/var/lib/protovoy/schedules.db` in that env file.
Leave it empty to keep scheduling disabled. Optionally set
`PROTOVOY_VIVA_SCHEDULE_TIMEOUT=30s` (positive duration, maximum `1h`).
The same writable data volume persists schedules and their bounded execution
summaries. Keep schedule, team and SQL execution databases in separate files.
See [schedule API examples](../../deploy/README.md#viva--go) for the required
login, Origin and scheduler headers.

```sh
curl --fail --user operator --output /dev/null --write-out '%{http_code}\n' \
  https://localhost:8443/healthz
```

Expect `204`. If the certificate uses a private CA, pass its certificate to curl
with `--cacert`. To run a container with
additional Go server flags, build the image from `apps/protovoy-go` and pass
the flags to the container entrypoint.

On SIGTERM or Ctrl+C, the server stops accepting connections and allows
ordinary in-flight HTTP requests up to eight seconds to finish. In the same
shutdown window, the WebSocket bridge rejects new upgrades and closes
connected clients with WebSocket status 1001 (Going Away). Pending upstream
handshakes are cancelled when shutdown begins.

### Go HTTP Mock Server

After pairing a Go Agent that advertises Mock API version 1, the **Mock rules**
panel also offers **Start Mock via Agent**, **Stop Mock via Agent** and
**Refresh Mock status**. Starting validates a snapshot of the editor content
and opens a dynamically assigned loopback port. The status message shows its
address. Stop the running instance before starting with edited rules.
The Agent stops its managed Mock when it receives SIGTERM or Ctrl+C.

In the Viva workbench, open **Mock rules** to edit a JSON array of scenarios.
**Validate rules** checks the document with the server's parser.
**Import Mock rules** validates a JSON file before replacing the editor content.
**Export Mock rules** saves `mock-rules.json` for the standalone service.
Invalid imports retain the current editor content. The editor keeps its contents
while switching panels during the current application session.

Create the example with `go run ./cmd/mock -rules mock-rules.json -init`.
The command rejects an existing destination. The resulting file contains a
JSON array of scenarios.

```json
[
  {
    "id": "greeting",
    "projectKey": "demo",
    "serviceKey": "api",
    "operationId": "greet",
    "method": "GET",
    "path": "/hello/{name}",
    "status": 200,
    "body": "Hello"
  }
]
```

```sh
go run ./cmd/mock -rules mock-rules.json
curl http://127.0.0.1:39218/m1/demo/api/hello/world
curl http://127.0.0.1:39218/m2/demo/api/greet
```

Run `go run ./cmd/mock -rules mock-rules.json -check` to validate the file
without starting a server. Invalid or missing files produce a nonzero exit code.
Starting with an explicit `-rules` path also requires a valid existing file.
With no `-rules` argument, the server watches `protovoy/mock-rules.json`
under the operating system's user configuration directory and can start
before that file is created.

The server binds to loopback. Use `/health` for its version and request count.
After a request attempts to reload invalid rules, `rulesReloadFailed` becomes
`true` in the health response. It clears after a subsequent successful reload.
Edit the rules file to update scenarios. The server keeps the last valid
rules while a replacement is incomplete. Unknown JSON fields are rejected,
including misspelled matching conditions. Set `delayMs` to delay a response
by up to 30 seconds or `errorEvery` to return HTTP 503 every Nth match.
For WebSocket scenarios, set `method` to `WS`, `wsMessages` to an array of
text messages and `wsIntervalMs` to the interval between them. Set `wsEcho`
to echo incoming text and binary messages after the preset messages.
Connections have a five-minute deadline and a 10 MiB incoming message limit.

### Go Local Agent

The native CLI manages system credentials with `protovoy secret set REFERENCE`,
`protovoy secret exists REFERENCE`, and `protovoy secret delete REFERENCE`.
The `set` command reads standard input and removes a single terminal LF or CRLF.
Values must contain 1 to 2560 bytes. Commands print status messages only.
These credentials use the `protovoy.go.secrets` system-store namespace.
Use `-header-secret 'Authorization: my-token'` on a CLI request to resolve the
complete header value from the system store. Save the `Bearer ` prefix as part
of the value when required by the API. Header secret options are repeatable.
After pairing with a Go Agent that advertises Secrets API version 1, the
**Local Agent** panel includes **System credentials**. Enter a reference and
masked value, then choose **Save secret**. Submission clears the value field.
**Check secret** reports existence and **Delete secret** removes that reference.
These fields stay outside workspace autosave and project exports.
Use `-password-secret REFERENCE` for SQL, Redis, MQTT, AMQP and Kafka CLI
requests. An explicit reference overrides that protocol's password environment
variable.

To use an HTTP credential in the workbench, enter its reference and masked
value in **System credentials**. Fill in the exact request URL and header name
under **HTTP credential binding**, then choose **Bind HTTP credential**.
Back in the HTTP editor, enter the same reference and header name and select
**Send via Local Agent**. The Agent checks the resolved URL, including its query
string, before applying the header. Turn off redirects and remove any manually
entered header with the same name. Workspace autosave retains the draft's
reference and header name. Saving a request also keeps its binding in the
workspace. Load a saved request to restore its own binding. Portable v2 project
exports carry the reference and header name. The Go collection runner resolves
the matching binding from its system keychain. cURL export requires removing
the binding.
On a CLI host, create the binding with
`protovoy secret bind-http REFERENCE URL HEADER`, supplying the complete
header value on standard input. Check or remove it with
`protovoy secret exists-http REFERENCE` or `protovoy secret delete-http REFERENCE`.

Run `go run ./cmd/agent` with an existing pairing token in
`PROTOVOY_AGENT_TOKEN` or supply its file with `-token-file`. It listens on
`127.0.0.1:39217` by default and accepts loopback binds only. If the Rust
Agent is running, use a different loopback port with `-bind`.
The default allowlist includes `http://127.0.0.1:39300`.
For other browser deployments, set their exact origin in `-allowed-origins` or
`PROTOVOY_ALLOWED_ORIGINS`.

In the desktop or browser workbench, select **Local Agent**, enter the
loopback address and pairing token, then select **Pair with Local Agent**.
Back in the workbench, enter a request URL and use **Send via Local Agent**.
HTTP, SSE, WebSocket, TCP, UDP, Redis, and GraphQL queries and subscriptions use the paired Agent. **Cancel**
stops an in-flight HTTP request. In SSE mode, **Connect via Local Agent**
opens the stream and **Disconnect** closes it. SSE retries preserve the last
event ID and respect the upstream retry interval. WebSocket and GraphQL
subscriptions request a short-lived, single-use ticket scoped to the workbench
origin and upstream setup. The token is sent in the WebSocket subprotocol
offer, and the Agent negotiates only the bridge protocol. Choose **Use default execution** in the Agent panel to
return to direct desktop execution or the browser server's configured
endpoint. The pairing token and session are kept out of workspace storage
and project exports.

For TCP and UDP, enter the destination and select **Send socket payload via
Local Agent**. TCP supports fixed-length and delimiter framing, verified TLS,
and optional private CA certificates. UDP returns one datagram, including a
valid zero-length response. The workbench displays reply bytes as Base64.

In the Redis panel, **Run Redis commands via Local Agent** uses the entered
endpoint and connection settings, including ACL credentials, database
selection and verified TLS. The Go Agent accepts the Redis v1 raw envelope
and returns ordered RESP2 or RESP3 values. Viva sends passwords with the
authenticated local execution request and keeps them out of workspace and
project files. Named password references are resolved by the Agent under its
configured TLS connection binding, with explicit keychain opt-in when needed.

The Agent also accepts MQTT 3.1.1 publish and subscribe requests through
`/v1/executions` with protocol ID `mqtt` and a `raw` payload. For example,
use `{"type":"raw","value":{"mode":"subscribe","topic":"demo/#","qos":2,"receiveLimit":3}}`
with an `mqtt://` or `mqtts://` broker endpoint. Publishing supports text
or Base64 payloads, QoS 0–2, and the retain flag. Subscriptions emit one
JSON record per message, including Base64 for binary payloads, and finish
at the receive limit. MQTT packets are bounded to 1 MiB and each execution's
result to 16 MiB. The authenticated payload accepts `username`, `password`,
`serverName`, and certificate-only `caPem` for verified MQTTS. Stored
secret references remain pending.

In Viva, select **MQTT**, then publish a text or Base64 payload or switch
to a subscription. Set the topic, QoS, and receive limit in the request
panel. Desktop can connect directly, and desktop and browser can use a
paired Local Agent. **Connection settings** provides client ID, username,
password, keep alive, clean session, and MQTTS certificate settings for
those paths. The hosted browser uses the server-configured broker when
the broker field is blank. Saving keeps the request and non-password
connection settings in the workspace and v2 project exports.

The Go Web server can also expose MQTT with
`-mqtt-target mqtts://broker.example.com:8883`. Set `-mqtt-username` and
`PROTOVOY_MQTT_PASSWORD` in the server process when the broker requires
credentials. Use `-mqtt-ca` for a private CA and `-mqtt-server-name` to
set the verified TLS name. The broker account's ACLs control available
topics. Each execution uses a fresh clean session. Redis, MQTT, AMQP, and Kafka
use a 30-second default timeout. Set **Timeout (ms)** in the workbench
or `timeoutMs` in the hosted bridge request to use 1–300,000 ms.
Custom timeouts survive legacy import and v2 export and apply to the
entire execution in desktop, hosted, and Local Agent modes.

`POST /api/mqtt` accepts `mode`, `topic`, `qos`, and the optional
`payload`, `encoding`, `retain`, and `receiveLimit` fields. An optional
`expectedTarget` must match the configured broker. The endpoint uses the
same hosted authentication, origin and preview-header checks as gRPC.
It streams newline-delimited `connected`, `message`, and terminal
`completed` or `failed` events. Cancelling the HTTP request closes the
broker connection.

The Go Local Agent accepts AMQP 0-9-1 through `/v1/executions` with protocol
ID `amqp` and a `raw` payload. Set the target to an `amqp://` or `amqps://`
endpoint with an optional virtual host, such as `amqps://broker.example.com/%2F`.
Use `username` and `password` in the authenticated payload for broker
credentials. For AMQPS, `serverName` and certificate-only `caPem` can
configure TLS verification.

Publish with `mode: "publish"`, `exchange`, `routingKey`, and a text or
Base64 `payload`. Mandatory publishing reports a broker confirmation or a
return for an unroutable message. Consume with `mode: "consume"`, `queue`,
and `receiveLimit`. Requests can declare a durable or transient exchange,
declare and bind a consumer queue, and select automatic acknowledgement
with `autoAck`. Manual acknowledgement is the default. Results stream as
one JSON record per delivery or publish result, with bodies bounded to
1 MiB and a 16 MiB result budget. Stored password references remain pending.

In Viva, select **AMQP** to publish or consume. **Request options** exposes
the exchange type, topology declarations, durability, acknowledgements, and
content type. For desktop direct and paired Local Agent execution, this
panel also accepts broker credentials and optional AMQPS trust settings.
The hosted browser uses the server-configured broker when its broker field
is blank. Saving preserves the request and non-password settings in the
workspace and v2 project exports. Passwords stay in the current session.

For hosted AMQP execution, set `-amqp-target amqps://broker.example.com/jobs`.
Use `-amqp-username` and `PROTOVOY_AMQP_PASSWORD` for broker authentication,
and optionally `-amqp-ca` and `-amqp-server-name` for verified TLS. The
target fixes both the broker and virtual host. The broker account's
permissions control topology, publishing and consuming.

`POST /api/amqp` accepts the publish and consume fields above and optional
`expectedTarget` to check the configured endpoint. It uses the same
authentication, origin and preview-header checks as `/api/mqtt`. It emits
the same newline-delimited event types and closes the AMQP connection
when the HTTP request is cancelled or the execution deadline expires.

The Agent exposes the protocol API v1 health, capabilities, session, HTTP
and SSE execution, TCP and UDP exchanges, Redis pipelines, execution event stream and
cancellation endpoints. A pairing token
can be exchanged for an eight-hour session scoped to the calling origin.
Execution events include response metadata, Base64 body chunks, an 8 KiB
preview, and a terminal result. HTTP request bodies support text and
Base64. The Agent accepts request variables and returns an error for
unsupported settings such as proxy overrides and disabled TLS
verification.

The Go Local Agent also accepts SQL through `POST /v1/executions`. Set
`protocolId` to `sql`, use a `postgres://host/database` or
`mysql://host/database` target, and send a `raw` payload. The payload's
`value` accepts `sql`, `parameters` (a JSON array), `transactional`,
`rowLimit`, `username`, and either a per-execution `password` or a
`passwordRef`. Use `$1` style
placeholders for PostgreSQL and `?` for MySQL. Query results include column
names, rows, and a truncation flag. Write results include affected rows and
the last insert ID when the driver exposes it. The default row limit is 500
and the maximum is 10,000. The execution timeout applies to connection and
statement execution together.

Both engines verify TLS certificates against system roots by default.
PostgreSQL also accepts explicit `?sslmode=disable` for local development.
Supply credentials in the payload. The Agent rejects credentials in URLs and
connection options that name local files.

For hosted SQL, configure the Go server with
`-sql-target postgres://db.example.com/protovoy -sql-username protovoy`.
Set `PROTOVOY_SQL_PASSWORD` in the server environment. MySQL targets use
`mysql://host/database` and the same credential settings. The SQL role
determines what authenticated workbench users can query or change.

Send `POST /api/sql` with the same origin and `X-ProtoVoy-Preview: 1`.
Use the same statement, parameter, transaction and row-limit fields as the
Agent's payload value. Omit `expectedTarget` to use the configured endpoint
or pass its exact value to check that the deployment matches the request.
The server owns the target, credentials and TLS settings. It returns
newline-delimited `connected`, `message`, and `completed` events, with
`failed` as the terminal event on execution errors. The bridge applies
a 30-second default execution timeout and accepts at most 16 concurrent SQL
requests. Override the deadline with `timeoutMs` from 1 to 300,000.
Select **SQL** in the Viva workbench to enter a statement, a JSON array of
parameters, a row limit, optional timeout and transaction mode. Use **Run SQL** for
desktop direct or hosted execution and **Run SQL via Local Agent** after
pairing. Leave the database URL blank to use the server-configured endpoint
in hosted mode. Desktop and paired mode accept a username and masked session
password. Saved requests and workspace autosave omit that password. Result
JSON appears below the editor as a bounded preview. Standalone legacy SQL
requests retain their password reference on import. Remove embedded passwords
before importing them.

For a named SQL password in desktop or Local Agent mode, set
`PROTOVOY_SQL_PASSWORD_REF`, `PROTOVOY_SQL_TARGET`, `PROTOVOY_SQL_USERNAME`,
and `PROTOVOY_SQL_PASSWORD` in the execution process environment. Enter the
reference in the workbench and leave the session password blank. The resolved
target must exactly match the configured URL. The request username can be
blank or match the configured username. Named passwords require verified TLS.
The execution process validates the binding and TLS settings before looking
up the password.
Each Go process currently supports one configured SQL password reference.
For a standalone CLI request, pass `-protocol sql -sql-password-ref REFERENCE`.
Collection runs read the reference from the saved request. Both modes support
`PROTOVOY_SQL_PASSWORD_KEYCHAIN=1` for keychain lookup. The standalone flag
cannot be combined with `-password-secret`.
For direct desktop execution, set `PROTOVOY_SQL_PASSWORD_KEYCHAIN=1`
before launching the workbench and save the value under that reference in
**System secrets** or with `protovoy secret set REFERENCE`. The desktop
workbench reads the keychain on each execution when this option is enabled.

To load the Agent's configured SQL password from the system keychain, save
the value with `protovoy secret set REFERENCE` and start the Agent with
`-sql-password-keychain`. Set `PROTOVOY_SQL_PASSWORD_REF`,
`PROTOVOY_SQL_TARGET` and `PROTOVOY_SQL_USERNAME` to bind the credential
to the verified TLS endpoint and username. Restart the Agent after rotation.

For hosted mode, set `PROTOVOY_SQL_PASSWORD_REF` in addition to the existing
server-side SQL connection settings. The browser can optionally send
`expectedPasswordRef` to check the configured credential name. The
database password stays in the execution process. Pass
`-sql-password-keychain` to load it at startup from the Go server user's
system keychain, using the configured reference. Restart the server after
rotating the password.

SQLite accepts `sqlite::memory:` for a fresh in-memory database per execution,
`sqlite:example.db` or `sqlite://example.db` for a relative path, and
`sqlite:///path/example.db` for an absolute path. The default `mode=rw`
opens an existing file. Append `?mode=ro` for read-only access or
`?mode=rwc` to allow creation. SQLite does not require database credentials.

Desktop mode opens the entered file directly. Paired browser clients can run
in-memory queries by default. To permit file access, start the Local Agent
with `-sqlite-target sqlite:example.db?mode=ro` and use that exact URL in
the workbench. For hosted mode, set `-sql-target` to the intended SQLite URL
and leave the workbench target blank. Relative paths are relative to the
process running SQLite. SQLite statements can inspect the database schema,
but cannot attach additional files, use `VACUUM INTO`, load extensions, or
change filesystem-related pragmas.

The Go Local Agent accepts Kafka via `POST /v1/executions` with
`protocolId: "kafka"`, a comma-separated `host:port` target, and a
`raw` payload. Its `value` requires a `topic`. For `mode: "produce"`,
set `payload` and optionally `encoding` (`text` or `base64`), `key`,
and `partition`. The result reports the broker-assigned offset and
partition. For `mode: "consume"`, set `groupId`, `offsetReset`
(`latest`, `earliest`, or `error`), and `receiveLimit`. Each emitted
record carries its offset, key, and payload. Manual mode commits each
emitted record. With `autoCommit: true`, only emitted records are
marked for automatic commits.

Kafka supports anonymous `PLAINTEXT`, verified `SSL`, and
`SASL_SSL` with `PLAIN`, `SCRAM-SHA-256`, or `SCRAM-SHA-512`.
Send credentials in the execution payload as `username` and `password`.
TLS accepts optional certificate-only `caPem`, and optional
`certificatePem` and `keyPem` for mutual TLS.

For hosted Kafka execution, set `-kafka-target kafka-1.example.com:9093,kafka-2.example.com:9093`.
Include every broker address the cluster advertises to clients. The server
only dials addresses on that list, including partition leaders and group
coordinators discovered after bootstrap. Set `-kafka-security SASL_SSL`,
`-kafka-sasl SCRAM-SHA-256`, and `-kafka-username` for SASL, with the
password in the server's `PROTOVOY_KAFKA_PASSWORD` environment variable.
For TLS, `-kafka-ca` adds trusted certificates. Mutual TLS uses
`-kafka-client-cert` and `-kafka-client-key` for the server's client identity.

Send `POST /api/kafka` from the same origin with `X-ProtoVoy-Preview: 1`.
The JSON body accepts the production and consumption fields above.
Omit `expectedTarget` to use the configured list or provide its exact value
to check the deployment. Broker addresses, credentials, and TLS settings
are configured on the server. The response uses newline-delimited
`connected`, `message`, and `completed` events, or a terminal `failed`
event. The server defaults to 30 seconds and allows up to 16 concurrent
Kafka requests. Set `timeoutMs` to override the execution deadline.
Select **Kafka** in the Viva workbench. Switch between produce and consume,
then set a topic and the corresponding message or consumer group options.
Desktop and paired Local Agent runs use the entered broker addresses.
Their **Connection settings** accepts SASL and optional CA certificates for
the current session. For mutual TLS, select **Load client certificate** and
**Load private key** to load a PEM certificate chain and an unencrypted PEM
private key. The workbench displays whether each file was loaded. Passwords,
CA input, and client identity files are omitted from saved requests.
Hosted runs can leave the broker field blank to use the configured server
addresses. The importer preserves legacy Kafka produce and consume settings
and password, CA, certificate and private key reference names from
standalone requests and v1 team snapshots. Encrypted private keys and
legacy key-password references still require manual migration.

For one named Kafka SASL password in desktop or Local Agent mode, set
`PROTOVOY_KAFKA_PASSWORD_REF`, `PROTOVOY_KAFKA_PASSWORD`,
`PROTOVOY_KAFKA_TARGET`, and `PROTOVOY_KAFKA_USERNAME` in the execution
process. Optionally set `PROTOVOY_KAFKA_SASL` (defaults to `PLAIN`) and
`PROTOVOY_KAFKA_CA_FILE` (defaults to system trust). Enter the reference
in **Connection settings**, select `SASL_SSL`, and leave the session
password and CA blank. The target must exactly match the configured list
and include each advertised broker the execution needs. Any request
username or SASL mechanism must match the configured value.
Desktop, CLI and Local Agent execution validate the broker addresses and TLS
material before looking up a named Kafka password.

For hosted mode, set `PROTOVOY_KAFKA_PASSWORD_REF` with the existing
server-side SASL configuration. The bridge checks the optional
`expectedPasswordRef` from the workbench against that name. The password
stays in the execution process in all three modes. For hosted mode,
`-kafka-password-keychain` loads the configured reference from the server
user's system keychain at startup after checking the broker and TLS settings.
Configure the broker and `SASL_SSL`,
and restart the server after rotating the password.

For Local Agent execution, start the Agent with `-kafka-password-keychain`
to look up the configured password reference in the system keychain on each
request. The same broker, username, SASL mechanism and TLS checks apply.
For direct desktop execution, set `PROTOVOY_KAFKA_PASSWORD_KEYCHAIN=1`
before launching the workbench. Both modes use the keychain in place of
`PROTOVOY_KAFKA_PASSWORD` when enabled. Save the password under the configured
reference in **System secrets** or with `protovoy secret set REFERENCE`.

For a standalone CLI request, pass `-protocol kafka -kafka-security SASL_SSL
-kafka-password-ref REFERENCE`. This flag conflicts with `-password-secret`.
Collection runs read the reference from the saved request. Both CLI modes
support `PROTOVOY_KAFKA_PASSWORD_KEYCHAIN=1` to use the system keychain.

For named TLS files in desktop or Local Agent mode, set
`PROTOVOY_KAFKA_TARGET` to the exact broker list. Pair each configured name
with its local PEM file using `PROTOVOY_KAFKA_CA_REF` and
`PROTOVOY_KAFKA_CA_FILE`, `PROTOVOY_KAFKA_CERT_REF` and
`PROTOVOY_KAFKA_CERT_FILE`, and `PROTOVOY_KAFKA_KEY_REF` and
`PROTOVOY_KAFKA_KEY_FILE`. Enter the names under **Connection settings**
→ **TLS references**. Certificate and key references must be supplied
together. Use `SSL` or `SASL_SSL` and leave the session CA blank.
For a standalone CLI request, use `-kafka-ca-ref`, `-kafka-cert-ref`
and `-kafka-key-ref` with those same configured names. Supply the client
certificate and private key references together. Use the configured CA
instead of `-ca` when passing credential references.
The execution process uses its configured trust and restricts broker
dials to the configured list.

For hosted mode, configure PEM files with the existing Kafka flags and
set the corresponding `PROTOVOY_KAFKA_*_REF` names. The bridge checks
any `expectedTlsReferences` from the browser against the configured
names. The browser does not send paths or PEM data for references.

After pairing, use **History** to see recent Local Agent executions. The
Agent retains the latest 200 finished executions and closed WebSocket relays
across restarts, including failures and cancellations. The records contain
IDs, protocol, state, HTTP status where available, timing, and response byte
counts. Request URLs, headers, and bodies are kept out of history. Response
capture is off by default. Set `-history-response-bytes` to a value from 1 to
1048576 to retain that many response bytes per execution. The **View captured
response** action displays up to 8 KiB as text or Base64, and the authenticated
`/v1/history/{id}/body` endpoint returns the retained bytes. Captures use
the same 200-record retention policy and can include sensitive upstream data.
WebSocket history records the session outcome, duration, and upstream payload
byte count. WebSocket messages are excluded from response capture.

Use `-history-dir` to choose the storage directory. The default
is `go-agent-history` under `PROTOVOY_CONFIG_DIR` or the per-user ProtoVoy
config directory. Executions from an unchanged saved Viva request carry a
local request ID. Select **Open saved request** in history to open its current
saved version, then send it again from the workbench. Older records and
unsaved drafts have no link. Local request IDs stay in the workspace and
are omitted from project exports. **Enable request snapshots** retains up
to 20 eligible saved templates in the local workspace, up to 256 KiB per
snapshot and 1 MiB total. Choose **Open historical request** to open that
version as a separate draft. Runtime credentials and resolved variables
are excluded. The draft uses the current global, collection and environment
values when sent. Requests with possible credentials are skipped. Snapshots
stay out of project exports and can be removed with **Clear request snapshots**.
The Go Agent exposes session pairing, protocol execution and cancellation,
retained history, WebSocket transport, and a single persisted workspace.
In **Local Agent**, pair with the Go Agent, then use **Create Agent workspace**
to store the current workbench state when no remote copy exists. Use
**Load Agent workspace** to apply the stored state. After loading or creating,
**Save Agent workspace** updates that version and **Delete Agent workspace**
removes it. A version conflict requires reloading before retrying.
Synchronization is explicit. The workbench also keeps its local autosave.
Loading a remote workspace applies its project, draft and variable scopes.
If the local state changes while loading, the workbench rejects the late result
and asks you to load again. Deleting the Agent copy retains the local workbench
state.

### Agent workspace API

All workspace requests use the Agent's existing loopback, Host, Origin and
Bearer authentication checks. Workspace data is stored under the configured
Agent history directory, with a maximum document size of 8 MiB.

| Request | Result |
| --- | --- |
| `GET /v1/workspace` | Workspace JSON and a strong `ETag`, or HTTP 404 when absent |
| `PUT /v1/workspace` with `If-None-Match: *` | Creates a workspace only if absent |
| `PUT /v1/workspace` with `If-Match: "<etag>"` | Replaces the version read by the client |
| `DELETE /v1/workspace` with `If-Match: "<etag>"` | Deletes the version read by the client |

Successful writes and deletes return HTTP 204. An unmet version condition
returns HTTP 412. Reload before retrying a conflicted change. The Go client
uses conditional requests for creation, saving and deletion. Raw callers may
omit conditions, which applies the operation to the current workspace.
Repeated unconditional deletion succeeds when the file is already absent.

Saved documents use workspace version 1. The server validates the document
and applies the same sensitive-data checks as local workbench autosave before
replacing the stored file. Invalid input returns HTTP 400 and leaves the
previous workspace available. Responses have `Cache-Control: no-store`;
allowed browser origins can read `ETag` and send version conditions.

### Go team collaboration API

Pass `-team-db /path/to/team.db` to the Go Web server to enable the
team API under `/api/team/v1`. Store the SQLite database in a persistent
directory writable by the server. To create the first organization and
owner, set `PROTOVOY_TEAM_BOOTSTRAP_TOKEN` to a secret of at least 24
printable characters before starting the server. Submit it as
`X-ProtoVoy-Bootstrap-Token` to `POST /api/team/v1/auth/bootstrap` with
`email`, `password`, `displayName`, and `organizationName`.
The server can run with the bootstrap token unset after initialization.

Use `POST /api/team/v1/auth/login` with `email` and `password` to
obtain a bearer token. Use that token for workspace reads, revision
updates, change queries and logout. In hosted mode the team API shares
the configured HTTPS origin and uses team bearer authentication for
workspace requests. The hosted workbench itself uses its configured
Web login.

An owner or admin can create an account with
`POST /api/team/v1/organizations/{organizationId}/members/provision`
using `email`, `displayName`, `password`, and `role`. Only owners can
create another owner. Organization members can read the roster at
`GET /api/team/v1/organizations/{organizationId}/members`.
Owners and admins can assign a role to an existing account with
`POST /api/team/v1/organizations/{organizationId}/members` using
`email` and `role`. Only an owner can change an owner's role, and each
organization must retain at least one owner.
Owners and admins can retrieve the latest 200 audit events at
`GET /api/team/v1/organizations/{organizationId}/audit`. Events record
successful initialization, member changes and workspace publishes.
Organization members can subscribe to
`GET /api/team/v1/organizations/{organizationId}/events` using a bearer
token. This SSE stream sends `workspace.changed` after a revision commits
and `comment.changed` after a comment is posted, edited, resolved or reopened.
After reconnecting, query the workspace or its change history to catch up.
Organization members can read or post comments and replies at
`/api/team/v1/organizations/{organizationId}/workspaces/{workspaceId}/comments`.
Authors, owners and admins can edit comments. Editors and higher roles can
resolve or reopen them. Comment changes are included in the organization audit.

In the workbench, open **Team workspace**, sign in, and enter a workspace
ID shared with your team. On desktop, enter the team server's HTTPS origin,
or an HTTP origin with a numeric loopback address for local testing. In the
browser, the workbench uses its own origin. **Publish saved project** creates an
empty workspace or updates the revision last loaded or published in this
session. **Replace local project from team** loads the current project and
collection variables. A stale publish requires loading the current revision
before retrying.
Select **Watch team updates** to receive live revision notices for the
entered workspace ID. Use **Replace local project from team** when ready
to apply an update. Leaving the team panel stops the subscription.
Use **Load workspace comments** to read or join the discussion for the
entered workspace ID. The comment controls follow the permissions of the
signed-in member. While watching the workspace, Viva indicates when the
discussion has changed so you can reload it when ready.

The hosted Go service can enable OIDC by setting
`PROTOVOY_TEAM_OIDC_ISSUER`, `PROTOVOY_TEAM_OIDC_CLIENT_ID`,
`PROTOVOY_TEAM_OIDC_CLIENT_SECRET`, and `PROTOVOY_TEAM_OIDC_ORGANIZATION_ID`
after team bootstrap. Set `PROTOVOY_TEAM_OIDC_DEFAULT_ROLE` to `VIEWER`
(the default), `RUNNER`, or `EDITOR`. Register
`https://<workbench-host>/api/team/v1/auth/sso/callback` with the provider.
The configured issuer must be trusted to verify account email addresses.
In the hosted browser workbench, choose **Sign in with SSO** in **Team
workspace**. The browser exchanges a short-lived HttpOnly handoff cookie
for the team session after returning from the identity provider.

Use **Load team members** to see the roster. Owners and admins can enter
an email and select a role, then choose **Assign role**. To create a new
account, also enter a display name and password and choose **Create member
account**.
Owners and admins can use **Load team audit** to review the most recent
organization activity in the workbench.

### Plugin execution and lifecycle

Use `protovoy plugin inspect PACKAGE_DIRECTORY` to inspect a package's
manifest, SHA-256 and publisher signature. Configure publisher keys with
`PROTOVOY_PLUGIN_TRUSTED_KEYS`, a JSON map of key IDs to Base64 Ed25519
public keys. For unsigned development packages, pass `-allow-unsigned`.
Grant declared permissions explicitly with repeated `-grant PERMISSION`.
This command checks package metadata and trust; `run` validates the Component
when loading and instantiating it.
Use `protovoy plugin install PACKAGE_DIRECTORY` with the same trust flags to
install a verified snapshot under the operating system's user configuration
directory, in `ProtoVoy/plugins/PLUGIN_ID` (Windows: `%AppData%/ProtoVoy/plugins`).
Set `PROTOVOY_PLUGIN_DIR` to choose a different store. Installation copies only
the manifest and Component, publishes the complete directory atomically, and
rejects an existing ID. Installation never executes guest code and does not
prove that a Component exports a runnable function. The JSON result includes
the installed directory and SHA-256. Use that directory with `inspect` or `run`;
trust policy and permissions are checked again on every invocation.
Use `protovoy plugin update PACKAGE_DIRECTORY` to switch an installed plugin
to a different version with the same ID, kind and publisher. The replacement
must pass the same trust and permission checks as an installation. The
plugin retains its enabled or disabled state. Previous snapshots stay on
disk until the plugin is removed so readers can finish using them.
Use `protovoy plugin list` to read installed manifest metadata from the same
store. The paired Viva workbench exposes **Refresh installed plugins** in the
Local Agent panel through authenticated `GET /v1/plugins`. Set
`PROTOVOY_PLUGIN_DIR` on the Agent to select its store. The inventory reports
broken manifests individually, ignores installation staging directories, and
does not read or execute Components or approve their signatures. Inventory is
limited to 1024 directory entries and a 1 MiB encoded response.
Select a listed plugin to run it from Viva. Enter its string input and explicit
comma-separated permission grants, then choose **Run selected plugin**. Signed
packages use the Agent's `PROTOVOY_PLUGIN_TRUSTED_KEYS`; unsigned development
packages require the explicit **Allow unsigned development plugin** option.
Selection resets that option and the grants. The authenticated
`POST /v1/plugins/{id}/run` accepts only IDs inside the configured store, checks
the stored manifest identity and trust again, and runs the verified snapshot.
The Agent admits one plugin execution at a time (HTTP 429 when busy). String
input and output are limited to 1 MiB; JSON bodies have a separate bounded
allowance for escaped text. Results from a previous Agent or plugin selection
are discarded by the workbench.
In the same panel, choose **Install plugin package**, then select `manifest.json`
and `plugin.wasm` from your computer. For directory-based installation and
execution, both package files must be regular files. Review the package identity and declared
permissions, enter explicit installation grants, and choose **Install on Local
Agent**. Unsigned development packages require a separate installation opt-in;
installation approval is not reused for execution. A new manifest resets the
selected Component and trust choices. Cancellation preserves the existing file
selection. Successful installation releases the selected file buffers.
Authenticated `POST /v1/plugins` uploads the manifest object and Base64 Component
bytes, never client filesystem paths. It limits files to 64 KiB / 32 MiB and the
JSON body to 46 MiB, reserves the plugin-operation slot before decoding, verifies
trust before writing, and uses the same atomic store publication as the CLI.
Duplicate IDs are rejected. Plugin install/run errors are returned to Viva as
bounded messages so signature, permission, and duplicate-package failures are
visible. Other Agent operations retain their existing error handling.
Paired clients can use `POST /v1/plugins/{id}/update` with the same upload
format. The package ID must match the route, and the Agent advertises
`pluginUpdates: true` in its health response when it supports this operation.
In Viva, refresh the installed plugins and open the same upload panel. If the
new manifest matches an installed plugin, the panel shows its current version
and offers **Update on Local Agent**. Select the new Component and grant its
permissions before updating.
When the paired Agent supports updates, Viva sends the selected version and,
when available, the installed snapshot revision with each run. If the plugin
has been updated or reinstalled, refresh the list and select it again before
granting execution permissions. Packages installed before revision tracking
use the version check until they are updated or reinstalled.
Use the inventory's **Disable NAME** / **Enable NAME** controls, or
`protovoy plugin disable PLUGIN_ID` / `protovoy plugin enable PLUGIN_ID`, to
change local execution policy. The authenticated
`POST /v1/plugins/{id}/state` requires an explicit `enabled` boolean and uses
the Agent's plugin-operation slot. State is stored as a `.disabled` marker
inside the rooted package directory. The inventory reports `disabled`, and both
installed-ID and direct-directory execution reject a disabled package before
loading its Component. New installations are enabled. Re-enabling does not
approve a signature or grant permissions. Viva clears the selected plugin's
previous grants, unsigned opt-in and output after a state change. Disabling
affects future executions; an already running CLI execution may finish. Agent
state changes return HTTP 429 while another plugin operation is active.
Use **Uninstall PLUGIN_ID** in the inventory or `protovoy plugin remove PLUGIN_ID`
to remove an installed package, including one with a broken manifest.
Authenticated `DELETE /v1/plugins/{id}` uses the same plugin-operation slot.
Removal accepts a validated ID inside the configured store, rejects package
directory symlinks, and atomically moves the package into a generated private
`.remove-...` directory before rooted recursive cleanup. Nested package symlinks
are not followed outside the store. If cleanup cannot finish, the response sets
`cleanupPending: true`; the package is already absent from the active inventory
and unavailable for new executions by installed ID. Interrupted cleanup can leave that hidden
directory on disk. Viva removes the item and clears its selected input, output,
grants and unsigned opt-in only after a successful response from the current
Agent. Other installed IDs and the original uploaded files are preserved. The
removed ID can be installed again as a new enabled package.
Use `protovoy plugin run PACKAGE_DIRECTORY` with the same trust flags to
execute the package's kind-specific string export. Input is read as UTF-8
from stdin and output is written to stdout. Execution has a two-second
deadline, a shared 32 MiB guest memory budget and bounded table and result
allocations. The runtime links no host capability imports.
For a standalone HTTP request, pass `-response-plugin PACKAGE_DIRECTORY`
to transform the UTF-8 response body (up to 1 MiB) before writing it to stdout.
The transformer must be signed by a configured trusted publisher and declare
no permissions. This option cannot be combined with HTTP scripts or
`-header-secret`, recognizable credential headers or query parameters, or URL user information.
The HTTP status remains on stderr. Bodies with recognizable credential fields
or private key markers are rejected before reaching the transformer.
For HTTP collections, `protovoy run -response-plugin PACKAGE_DIRECTORY PROJECT.json`
uses one verified snapshot throughout the run and checks body assertions against
the transformed text. Requests with scripts or credential bindings are rejected
before execution.
Use `protovoy plugin import PACKAGE_DIRECTORY < document.txt > project.json`
to run an importer and write a portable ProtoVoy project. The command reads
UTF-8 input up to 1 MiB and applies the same trust flags as `plugin run`.
The plugin must return a ProtoVoy v1 or v2 project JSON document. The host validates it
and checks it for recognizable credentials before writing the project.
Importer output cannot include request scripts. Add any scripts you intend to
run after importing the project.
In Viva, select an installed importer, run it with the document as input,
then choose **Import plugin result** to load the validated project. Export
any unsaved changes to the current project before importing.

The plugin execution contract remains the Component worlds in
`plugins/sdk/protovoy-plugin.wit`, including string input and output.
The inspected `wasmtime-go/v50 v50.0.0` exposes Component compilation and
instantiation, while its Component function calls and value marshaling remain
unimplemented. Integrating execution requires a binding that supports these
calls together with memory limits, interruption and capability isolation.
Package inspection currently performs no Component compilation or execution.
The inspected pure-Go candidate `github.com/kjanat/wacogo v0.2.3` offers
Component string calls and configurable wazero resource limits. It requires
Go 1.27, beyond this module's Go 1.26 toolchain, and remains under evaluation.
An isolated Go 1.26 probe successfully initialized `wacogo v0.1.0` with
a 32 MiB guest memory limit and context interruption enabled. This earlier
version also passed an isolated `transform(input: string) -> string`
Component probe for ASCII, Chinese and empty strings, including repeated
calls on the same instance. Further isolated probes verified interruption of
a guest infinite loop with a 100 ms context deadline and rejection of a
513-page memory declaration under a 512-page limit. It remains a candidate
pending malformed-component tests, aggregate resource limits and review of
fixes in later releases.
The shared-allocation probe blocked a second 32 MiB module within a 32 MiB
component budget. Allocation denial during instantiation panics in the
candidate's wazero dependency, so the probe catches the panic at its
instantiation boundary. Closing the engine released all accounted memory.
Production integration still requires this failure path to become a stable
error and requires equivalent limits for tables and canonical lifting.
The table-admission probe rejected unbounded tables and rejected two
600-entry maxima under a shared 1,000-entry budget. Engine shutdown refunded
all reserved entries after failed instantiation. Canonical result lifting
budgets and production runtime integration remain to be verified.
The canonical result probe accepted a small string and returned
`ErrLiftLimit` for a 256-byte result under a 128-byte lifting budget.
The Go plugin module now uses the candidate runtime behind
`ExecutePackage`, which reads and verifies a fresh package before calling its
kind-specific string export. Integration tests cover ASCII, Chinese and empty
strings, trust policy, kind mismatches, input limits and cancellation.
CLI and paired Viva execution and installation are integrated. Review of fixes
introduced after the selected runtime version remains pending.

The Go CLI accepts `-pre-script path.js` and `-post-script path.js` for
individual HTTP requests. Scripts can use `request.target`, `request.payload`,
`variables`, `assert` and `crypto`. Post-response scripts also receive
`response` and run before output is written. Script files are limited to
64 KiB and post-response bodies to 1 MiB. HTTP script flags cannot be combined
with `-header-secret`.
With `protovoy run`, the same flags apply to each HTTP request in the
collection. Scripts receive the iteration, collection and request variables
for that request. Failed post-response assertions mark the collection result
as failed while retaining the HTTP status.
HTTP requests in a v2 project can also carry `preScripts` and
`postScripts` arrays. The CLI runs command-line pre-scripts before saved
pre-scripts, and saved post-scripts before command-line post-scripts.
Legacy HTTP request envelopes retain their scripts on import. In Viva,
open **Request scripts** on an HTTP request to edit each phase as a JSON
array of JavaScript strings. Run scripts directly in the desktop HTTP workbench
or send them from the browser through the Go Web server without pairing an
Agent. A paired Go Local Agent is also supported from desktop and browser.
Scripts and current variable values are sent through the authenticated execution
API. HTTP credential bindings cannot be combined
with scripts. Projects can also be executed with the Go CLI.

Hosted `/api/preview` accepts `preScripts`, `postScripts` and `variables` for
non-streaming HTTP requests, under the same authentication, origin and preview
header checks as ordinary requests. Both the initial URL and script-modified
URL must exactly match a configured HTTP endpoint; scripts cannot enable
redirects or use additional HTTP methods. The server admits two scripted
executions at a time (HTTP 429 when busy), using the existing QuickJS sandbox
and 500 ms limit per phase. Ordinary HTTP execution does not consume these
script slots. Post-response scripts buffer at most 1 MiB before returning a
preview. Script failures return HTTP 422 and a fixed phase indicator, without
upstream response metadata, body content or script exception text.
Non-UTF-8 request and response bodies are exposed to scripts as Base64 strings
with `bodyEncoding: "base64"`; raw bytes are preserved on the HTTP transport.
Pre-request scripts can deliberately change the encoded body, or replace it
with text and clear `bodyEncoding`. Unknown or invalid encodings are rejected.

The following product areas still require implementation or integration before
the Go and Viva rewrite can replace the full application.

The OIDC backend also supports a desktop authorization-code handoff. Login may
include `desktopReturn` and `desktopChallenge`; the return URL is restricted to
`http://127.0.0.1:PORT/team-sso?state=STATE` on an unprivileged port, with a
32-byte Base64URL state and SHA-256 proof challenge. Provider callback handling
still verifies the signed ID token, nonce and verified email. It redirects only
a random, 60-second, single-use code to loopback, without a team token in the
URL or browser handoff cookie. A native client exchanges that code and its
32-byte Base64URL verifier through HTTPS `POST /api/team/v1/auth/sso/desktop/session`.
The verifier is hashed as its encoded ASCII string, as with S256 PKCE. Desktop
exchanges reject browser Origin / Fetch Metadata headers, unknown JSON fields,
oversized bodies, wrong proofs, expiry and replay. Each service instance retains
at most 128 pending grants in memory; restarts invalidate pending handoffs.
SSO configuration advertises `desktopEnabled`. In Viva desktop, open **Team
workspace**, enter the HTTPS team-server origin and choose **Sign in with SSO**.
The system browser opens the configured provider. The app listens only on a
random `127.0.0.1` port, checks the exact Host, path and state, rejects browser
XHR origins and repeated callbacks, then closes the listener after exchanging
the code. **Cancel team operation** and the three-minute sign-in timeout also
close it. The team token remains in memory. A server without desktop SSO support
is reported before opening the browser. Native-client integration tests exercise
a signed local TLS OIDC provider, the real loopback listener and HTTPS exchange;
deployment against an organization's identity provider still needs verification.

| Area | Current Go and Viva state | Remaining work |
| --- | --- | --- |
| Remote gateway and schedules | Hosted fixed-target execution bridges, persisted recurring jobs, bounded execution history, CLI runs, Viva controls and opt-in bound HTTP credentials | Hosted runtime/CI deployment verification |
| Secrets | System keychain management, saved MQTT/AMQP/Redis password references, CLI password references, and Agent-backed HTTP header bindings | Credential-reference integration for remaining protocols |
| Request authentication | Bound HTTP headers, OAuth2 exchange core, OS-keychain OAuth bindings, context-aware collection/schedule resolution and deferred Agent credential resolution | Add Viva OAuth configuration controls and verify deployed provider integration |
| Collaboration | Viva desktop and browser can sync projects, receive updates, manage members, view audit events and discuss workspaces; hosted browser and desktop support OIDC sign-in | Deployment verification against an organization's identity provider |
| Extensibility | Go QuickJS HTTP scripts in desktop, hosted browser and Agent; bounded execution for all four SDK Component worlds; CLI and paired Viva plugin lifecycle controls including updates | Plugin host capability bindings, request-pipeline integration, and runtime dependency review |

Historical user-data migration is outside the rewrite scope while the product
is in development.

The request-authentication behavior reference is
`crates/execution-engine/src/auth.rs`. This includes Basic, Bearer, API key,
OAuth2 client credentials and OAuth2 authorization code. The new
`internal/execution.OAuthClient` exchanges both OAuth2 grants using runtime-only
credentials, HTTPS or loopback HTTP endpoints, bounded responses and deadlines.
Redirects are disabled, token values are checked before use as headers, and
endpoint error bodies are not propagated. Local TLS fixtures exercise grant
parameters and client authentication. Native Agent and CLI integration tests
also pass; dedicated Viva OAuth configuration controls are still pending.

OAuth bindings now reuse the existing `httpSecretReference` and
`httpSecretHeader: "Authorization"` request fields. Exchange inputs live in the
OS credential store, bound to the exact resource URL and a fixed token endpoint.
`protovoy secret bind-oauth-http REFERENCE RESOURCE_URL` reads one JSON document
from standard input, with the `OAuthTokenRequest` fields `Endpoint`, `GrantType`,
`ClientID`, `ClientSecret`, optional `Scope`/`Audience`, or `Code`, `Verifier` and
`RedirectURI` for authorization code. Standard input is limited to 2560 bytes;
unknown fields and trailing JSON are rejected. Do not put this credential JSON
in a workspace or tracked file. Collection execution and opted-in scheduled HTTP
use a context-aware resolver. Resource/header mismatches are rejected before
contacting the provider, and cancellation stops the exchange. Agent preparation
now checks the binding without network access, then resolves credentials inside
the registered job using its cancellation/deadline context. The context-free
helper still rejects OAuth bindings; static header bindings retain their
existing behavior. New Agent regression tests cover token injection and
cancellation before reaching the resource endpoint and now pass locally.

On 2026-11-23, formatting and `node scripts/validate-changed.mjs --go` passed,
including the OAuth CLI end-to-end test, input-size regression, Agent lifecycle
tests, native Go suite and Viva WebAssembly compile check. The earlier Windows
application-control restriction no longer blocked this validation run. This
does not establish remote CI, Docker runtime or deployed-provider verification.

The schedule behavior reference is `apps/protocol-gateway/src/main.rs` and
`packages/ui/src/GatewayWorkbench.tsx`. It includes job creation, listing
and deletion, an interval of at least ten seconds, enabled state, next-run
time, latest execution ID, persisted jobs, and a bounded recent execution
history. These remain part of the rewrite completion criteria.

`internal/scheduler` now provides SQLite job persistence, enable/disable
transitions, non-overlapping bounded workers, and the latest 500 execution
summaries without response bodies or headers. Its HTTP handler supports
`GET/POST /v1/jobs`, `PATCH/DELETE /v1/jobs/{id}`, and
`GET /v1/executions`; it denies every request unless the host supplies an
authorizer. JSON writes reject unknown fields, trailing documents, oversized
input, and credential-bearing requests. The server mounts these routes only
when `-schedule-db <filesystem-path>` is supplied. Compose exposes the database
and timeout settings. The Viva Schedules panel connects to these routes, creates
disabled jobs from selected saved requests, refreshes jobs and execution
summaries, and supports enable, disable and delete operations. Request variables
override saved collection variables in each new job snapshot. Unsaved draft
edits and the active environment are not included.

`RunRecorded` connects the worker loop to durable execution summaries, enforces
a configurable execution timeout of at most one hour, and records interrupted
attempts during shutdown with a separately bounded database write. Protocol
adapters return assertion success and status only; executor errors and panic
payloads are not retained or forwarded to infrastructure logging. Adapters must
honor the execution context so shutdown can drain active workers.

The server's `openScheduledService` helper owns the database and recorded worker
lifecycle. Its API wrapper uses existing Host, HTTPS and hosted Basic login
checks, and requires `X-ProtoVoy-Scheduler: 1` on every request. Mutations require
the exact configured Origin. Browser GET requests that omit Origin must carry
the browser-controlled `Sec-Fetch-Site: same-origin` header; explicit mismatched
Origins are rejected. Closing the helper cancels and drains
workers before closing SQLite; a regression test reopens the database to verify
the interrupted attempt was retained. The main startup path invokes this helper
when schedules are enabled. `-schedule-timeout` defaults to `30s` and accepts a
positive duration up to one hour. The schedule database must be a persistent
filesystem path separate from the team database, SQL execution target and their
SQLite sidecars; hard-linked aliases are rejected as well. The scheduler uses
the same configured protocol targets as interactive execution.

The first scheduled protocol adapter prepares HTTP, GraphQL, JSON-RPC and SOAP
requests through the existing project helpers. CLI and scheduled execution now
share response-body matching and protocol fault detection in
`internal/execution/collection_response.go`. Scheduled HTTP requests must match
an exact configured target and cannot follow redirects. Expected status and
variable-resolved body assertions are checked; invalid assertions are rejected
before sending. HTTP pre-request and post-response scripts use the existing
QuickJS sandbox. The adapter revalidates the target, method and redirect policy
after pre-request scripts, and preserves variables for response assertions.
Script failures expose only a generic phase error. Scheduled HTTP credential
injection is disabled by default. A native server with system-keychain access
can opt in with `-schedule-http-keychain` (requires `-schedule-db`). Create an
HTTP binding under the same operating-system account with
`protovoy secret bind-http REFERENCE URL HEADER`, supplying the secret on standard
input. Saved jobs contain only `httpSecretReference` and `httpSecretHeader`.
The adapter validates the resolved target against its allowlist before resolving
the exact URL/header binding. Credential-bound requests cannot use scripts,
redirects or an already-present credential header. Binding failures return a
generic error and no credential value is retained in job or execution history.
The Docker image does not configure a host system-keychain service; this native
opt-in flag is not enabled by the Compose template.

The scheduled TCP/UDP adapter resolves request variables, checks the exact
configured target for the selected network, validates timeout and repeated-send
limits, and delegates to the shared socket executor. Binary payloads and TCP
framing use the same implementation as interactive requests; UDP retains
datagram boundaries. Regression tests exercise real loopback TCP and UDP peers.

The scheduled SQL adapter reuses the server-configured database endpoint and
checks the resolved target and password reference before preparing statements.
It supports existing SQL variables, parameters, transactions, row limits and
execution deadlines. Query results are discarded rather than stored in schedule
history. SQLite integration tests cover actual writes and reads, rejected-input
side effects, and interruption of a long-running recursive statement.

The scheduled Kafka adapter reuses the configured hosted broker and the same
password/TLS reference checks as the interactive bridge. It resolves target,
topic and payload variables, supports produce and bounded consume operations,
and applies the request deadline without retaining message contents in execution
history. A local Kafka protocol fixture verifies binary production, consumption,
and rejection of mismatched targets, credential references and TLS references.

The scheduled MQTT adapter shares preparation and credential-reference checks
with the interactive bridge. Server-owned credentials, TLS configuration and
fresh clean sessions are used for each invocation. Target, topic and payload
variables, publish/subscribe options and execution deadlines are supported.
A verified TLS protocol peer checks credentials, binary subscription delivery
and graceful DISCONNECT after the receive limit, separately from cancellation.

The scheduled AMQP adapter shares server-owned broker preparation and password
reference validation with the interactive bridge. It resolves destination and
payload variables, preserves declaration and consume settings, applies execution
deadlines and requires an explicit successful publication confirmation for
publish jobs. An AMQP wire-protocol peer verifies the configured credentials,
virtual host, binary publication and broker acknowledgement.

The scheduled Redis adapter executes variable-resolved commands against the
configured server endpoint and uses its TLS, ACL and database options. Named
credential assertions must match the configured password reference, username,
TLS name and numeric database index. Redis error replies fail the job without
retaining their diagnostic text. Loopback RESP fixtures verify successful PING
and error responses; binding tests cover mismatches and the TLS requirement.

The scheduled gRPC adapter uses the shared gRPC client for binary and
descriptor-based JSON messages. It resolves request metadata and variables,
requires the exact configured endpoint, and evaluates transport status together
with gRPC trailers and execution errors. HTTP/2 regression fixtures cover a
successful stream, nonzero gRPC status, timeout and rejection before dialing an
unconfigured target. Message contents are not retained in schedule history.

The scheduled SSE adapter uses the shared decoder and reconnect loop with exact
HTTP target checks and redirects disabled. It preserves Last-Event-ID across
reconnections, finishes after EOF and the configured reconnect attempts, and
interrupts persistent streams at the request deadline. There is no receive-count
setting in the current SSE contract. Regression tests cover reconnection,
deadline cancellation and invalid content types without retaining event bodies.

The scheduled WebSocket adapter and CLI share the native automation loop for
initial frames, reconnects, idle timeouts and cumulative receive limits. The
adapter resolves variables before enforcing the exact configured target and
fails a job when the connection closes before its requested receive count.
The scheduler execution context bounds the overall session. Real WebSocket
fixtures cover binary frames resent after reconnection and early-close failure;
received message bodies are discarded from schedule history.

The scheduled GraphQL subscription adapter and CLI share the native
graphql-transport-ws connection lifecycle. The adapter enforces the configured
WebSocket target after variable resolution and fails rejected operations or
`next` payloads containing GraphQL errors. Its overall lifetime is bounded by
the scheduler context. WebSocket fixtures cover successful completion, operation
rejection, GraphQL data errors and deadline cancellation; payloads are discarded
instead of being written to schedule history.

`scheduledExecutor` now joins the protocol adapters behind one server execution
entry point. It validates the complete portable request before dispatch, rejects
mixed protocol documents and inline credentials, and sends unconfigured
protocols to their own rejecting adapter instead of falling through to HTTP.
Its HTTP allowlist is captured at construction. Integration tests exercise HTTP
and SQL dispatch and verify invalid requests produce no HTTP side effects.

`internal/scheduleclient` provides the portable schedule management client for
the Viva UI. It shares job/result wire models with the scheduler, supports the
complete management API, restricts server origins to HTTPS or loopback HTTP,
and does not follow redirects. Native hosted credentials remain in memory;
browser clients use the same-origin browser login. Every request carries the
Origin and scheduler marker, and response decoding is bounded to 16 MiB.
SQLite-backed API tests cover management operations and credential rejection;
additional tests ensure redirects and remote error text are not propagated.

The Linux HTTPS Compose smoke workflow now enables a separate schedule database
and checks authenticated API access, persistence of a disabled SQL job across
container replacement, enablement, actual execution, its latest execution ID,
disablement and deletion. These are CI runtime assertions, not a record of a
successful deployment run. Local workflow YAML parsing and Bash syntax checks
pass; Docker runtime verification requires a running Docker engine.

`node scripts/smoke-schedules.mjs [bundle-directory]` exercises the Viva schedule
panel in headless Chrome using a real local server and HTTP fixture. The default
bundle is `build/native` with a `server` executable and `web` assets. Set
`CHROME_BIN` for a nondefault browser path, `PROTOVOY_SCHEDULE_SMOKE_PORT` to
override port 39317, and `PROTOVOY_SCHEDULE_SCREENSHOT` to save a screenshot at a
chosen path. The script creates an isolated schedule database, enables a job,
waits for an actual successful execution, refreshes the visible summary, then
disables and deletes the job. Linux CI runs this after the existing browser
collection-assertion smoke test. Local execution of this script passes; remote
CI execution remains unverified until the workflow runs.

The Rust and TypeScript versions remain the reference for behavior. Before
switching the default entry points, verify the existing request and event
contracts, workspace import and export, cancellation and streaming, and the
Agent's loopback binding, pairing, authorization, and origin checks. Keep the
server execution API separate from the local transport policy.

For a Viva gap, first reproduce the issue in a minimal upstream example and
record the affected build targets. Keep ProtoVoy-specific logic in this
repository and propose reusable fixes upstream.
