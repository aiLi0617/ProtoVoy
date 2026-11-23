# ProtoVoy private deployment

## Viva + Go

Use [viva.compose.yaml](viva.compose.yaml) to serve the browser workbench
over HTTPS. Copy [viva.env.example](viva.env.example) to `deploy/.env.viva`
and fill in the HTTPS origin, password file, and certificate paths. From
the repository root:

```bash
docker compose --env-file deploy/.env.viva -f deploy/viva.compose.yaml config --quiet
docker compose --env-file deploy/.env.viva -f deploy/viva.compose.yaml up --build -d
curl --fail --user operator --output /dev/null --write-out '%{http_code}\n' https://localhost:8443/healthz
```

Use your configured origin and username in the health check. Curl prompts
for the password. A ready server returns HTTP 204.

Set `PROTOVOY_VIVA_TEAM_DB=/var/lib/protovoy/team.db` to enable collaboration.
For SQL request testing with a writable SQLite file, set
`PROTOVOY_VIVA_SQL_TARGET=sqlite:/var/lib/protovoy/jobs.db?mode=rwc`.
Use separate files for collaboration and SQL requests. At startup, the
server rejects overlaps between a file-backed SQLite target and `-team-db`,
including their SQLite log and shared-memory files. The
`team-data` volume keeps both files across container replacements.
See the [Viva deployment guide](../apps/protovoy-go/README.md#docker-compose-deployment)
for initial team setup, protocol targets, and private CA configuration.

Set `PROTOVOY_VIVA_SCHEDULE_DB=/var/lib/protovoy/schedules.db` to enable
recurring jobs. `PROTOVOY_VIVA_SCHEDULE_TIMEOUT` defaults to `30s` and accepts
positive Go durations up to `1h`. The existing `team-data` volume persists the
schedule database across container replacements. Keep it separate from the
team database and SQL execution database, including their SQLite sidecars.
Scheduling remains disabled when the path is empty.

The Go server exposes `GET/POST /v1/jobs`, `PATCH/DELETE /v1/jobs/{id}` and
`GET /v1/executions`. Requests require the configured HTTPS login and
`X-ProtoVoy-Scheduler: 1` header. Mutations require the exact configured Origin;
browser GET requests may instead carry `Sec-Fetch-Site: same-origin` when Origin
is absent. For example, after setting an
allowed HTTP target, create a disabled job for review:

```bash
curl --fail --user operator https://localhost:8443/v1/jobs \
  -H 'Origin: https://localhost:8443' -H 'X-ProtoVoy-Scheduler: 1' \
  -H 'Content-Type: application/json' \
  --data '{"name":"Health check","intervalSeconds":60,"enabled":false,"request":{"method":"GET","url":"https://api.example.com/health"}}'
```

Replace the URL with an exact configured endpoint. Enable the returned job ID
with `PATCH /v1/jobs/{id}` and `{"enabled":true}` using the same headers.
Execution history retains the latest 500 summaries without response contents.
The Viva **Schedules** panel supports connection, creation from a selected saved
request, enable/disable, deletion and recent execution summaries. New jobs start
disabled for review. Desktop users enter the server origin and hosted login;
browser users connect to the current server using their existing browser login.

## Existing Web stack

This deployment runs the complete Web stack behind one Nginx entry point:

- Web UI at `/`
- authenticated Rust execution Agent at `/agent`
- authenticated multi-protocol cloud Gateway at `/gateway`
- Java collaboration/OIDC service at `/collaboration`
- PostgreSQL on the internal Docker network only

Only the Web port is published. Agent SQLite/workspace data, Gateway schedules and PostgreSQL collaboration data use named volumes.

## Start

```bash
cp deploy/.env.example deploy/.env
# Replace every required value in deploy/.env before continuing. The bootstrap token may stay empty.
docker compose --env-file deploy/.env -f deploy/compose.yaml config --quiet
docker compose --env-file deploy/.env -f deploy/compose.yaml up --build -d
docker compose --env-file deploy/.env -f deploy/compose.yaml ps
```

Open `http://localhost:8080` (or `PROTOVOY_PUBLIC_ORIGIN`). If `PROTOVOY_BOOTSTRAP_TOKEN` is empty, read the generated first-owner token once from `docker compose logs collaboration`, then use Team → 首次引导. You may still preconfigure a high-entropy token. The browser uses runtime `config.js`, so changing the public paths or Agent token only requires recreating the Web container, not rebuilding its image.

## Production requirements

- Put the Web port behind an HTTPS ingress or load balancer and set `PROTOVOY_PUBLIC_ORIGIN` to the exact external origin.
- Use independently generated high-entropy values for the database password, Agent token, and Gateway API key. The collaboration service may generate the short-lived bootstrap token at startup.
- Restrict ingress access: the runtime Agent token is delivered to authenticated users' browsers and should be treated as an internal deployment credential.
- Back up `postgres-data`, `agent-data`, and `gateway-data`; test restore before upgrades.
- For OIDC, register `https://your-host/collaboration/login/oauth2/code/oidc` and complete the optional variables in `.env` after the first organization exists.

## Verify and stop

```bash
curl --fail http://localhost:8080/healthz
curl --fail http://localhost:8080/agent/health
curl --fail http://localhost:8080/collaboration/actuator/health
curl --fail http://localhost:8080/gateway/health
curl --fail -H "Authorization: Bearer $PROTOVOY_GATEWAY_API_KEY" http://localhost:8080/gateway/v1/capabilities
docker compose --env-file deploy/.env -f deploy/compose.yaml down
```

`down` preserves named volumes. Only use `down --volumes` when permanent data deletion is explicitly intended.

## Gateway API

- `POST /gateway/v1/executions` executes one `RequestEnvelope` remotely and returns its complete event stream.
- `POST /gateway/v1/runner/execute` returns a CI-oriented `exitCode`; `failOnAssertion` makes failed assertions return exit code 1.
- `GET/POST /gateway/v1/jobs` and `DELETE /gateway/v1/jobs/{id}` manage persistent interval schedules (minimum 10 seconds).
- `GET /gateway/v1/executions` exposes the latest 500 in-memory summaries; response bodies, logs, extracted variables, and response headers are removed from retained history.

All `/v1` endpoints require `Authorization: Bearer <PROTOVOY_GATEWAY_API_KEY>`. The standalone Gateway binds to `127.0.0.1:39219` by default; Compose explicitly binds it to all container interfaces. Scheduled requests are persisted in `gateway-data`. The Gateway rejects recognizable inline credentials in scheduled requests, including sensitive headers, URL query keys, and structured fields. It does not currently resolve secret references, so credentialed schedules are unsupported. Keep sensitive values out of free-form bodies and scripts. If an older `jobs.json` contains inline credentials, startup fails until the affected jobs are removed; rotate those credentials before restarting. The capability endpoint describes request routing and retention behavior.
