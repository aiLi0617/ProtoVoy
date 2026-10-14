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
  "variables": { "baseUrl": "https://staging.example.com" }
}
```

The response contains ordered `cases` and `exitCode`. `exitCode` is `1` when any request, assertion, or variable resolution fails; otherwise it is `0`. Variables extracted by post-scripts are available to later requests in the same run only and are not persisted by the Gateway.
