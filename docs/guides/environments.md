# Environments and variables

ProtoVoy supports scoped variables resolved at execution time with `{{name}}` syntax.

## Scopes

| Scope | Typical use |
|-------|-------------|
| Global | Defaults across projects |
| Environment | `dev`, `staging`, `prod` base URLs |
| Collection | Shared variables for a folder |
| Request | Overrides for a single request |

## Example

Environment variable:

| Name | Value |
|------|-------|
| `baseUrl` | `https://httpbin.org` |

Request URL:

```text
{{baseUrl}}/get
```

## Dynamic helpers

Built-in helpers include `$uuid`, `$timestamp`, and `$isoTimestamp` (see execution engine docs in code).

## Global request parameters

Global parameters are persisted with the default environment and can inject defaults into HTTP requests in four locations:

| Location | Behavior |
|----------|----------|
| Header | Adds a header when the request does not already contain that header name (case-insensitive). |
| Cookie | Appends a cookie when the request's `Cookie` header does not already contain that cookie name. |
| Query | Adds a URL query pair when the request does not already contain that key. |
| Body | Adds a JSON property, form-urlencoded field, or multipart text field when that field is absent. |

Names and values support `{{name}}` resolution. Disabled rows are retained but not injected. Request-local values always take precedence, and global request parameters apply only to HTTP requests.

## Git-friendly exports

Environment definitions can be exported in ProtoVoy project packages (`*.protovoy.json`). Use `secret_ref` for sensitive values — see [secrets.md](./secrets.md).
