# CLI collection runs

Use `protovoy-cli run` to execute collections in CI.

## Minimal collection

See [examples/collections/httpbin-smoke.json](../../examples/collections/httpbin-smoke.json).

## Run

```bash
protovoy-cli run examples/collections/httpbin-smoke.json --report report.json
```

## Options

| Flag | Description |
|------|-------------|
| `--concurrency N` | Parallel cases (default 1) |
| `--fail-fast` | Stop on first failure |
| `--report PATH` | Write JSON report |
| `--report-format junit` | JUnit XML for CI dashboards |
| `--data PATH` | JSON array or CSV for data-driven iterations |

## Exit codes

Non-zero exit indicates one or more failed cases or assertions — suitable for GitHub Actions and other CI systems.

## Portable vs full envelopes

Collections may be:

- An array of `{ "name", "url", "method", "headers", "body" }` portable objects, or
- Full `RequestEnvelope` JSON objects with `protocolId`

## Run through Protocol Gateway

For a CI worker that cannot reach the target network directly, send expanded full envelopes to the authenticated Gateway collection endpoint. The existing single-request endpoint remains compatible.

```http
POST /v1/runner/collection
Authorization: Bearer <gateway-api-key>
Content-Type: application/json

{
  "requests": [/* full RequestEnvelope objects in execution order */],
  "failFast": true,
  "variables": { "baseUrl": "https://staging.example.com" },
  "data": [
    { "userId": "alice" },
    { "userId": "bob" }
  ],
  "reportFormat": "junit"
}
```

`data` is optional. When present, each object runs the full collection as one numbered iteration; row values override same-named base `variables`. `reportFormat` accepts `json` (default) or `junit`.

The response contains ordered `cases` with `iteration`, `exitCode`, `reportFormat`, and the rendered `report`. `exitCode` is `1` when any request, assertion, or variable resolution fails; otherwise it is `0`. With `failFast`, the Gateway stops the current collection and does not start later iterations after a failure. Variables extracted by post-scripts are available to later requests in the same iteration only and are not persisted by the Gateway.
