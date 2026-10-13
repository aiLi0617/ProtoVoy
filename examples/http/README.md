# HTTP examples

Public endpoints only — no credentials.

## Simple GET

Import or recreate in the HTTP workbench:

- File: [get.http.json](./get.http.json)
- URL: `https://httpbin.org/get`
- Method: GET

## With environment variable

See [with-env.protovoy.json](./with-env.protovoy.json) — uses `{{baseUrl}}` resolved from an environment.

## With assertion

See [with-assertion.protovoy.json](./with-assertion.protovoy.json) — expects `status == 200`.

## CLI

```bash
protovoy-cli http-get https://httpbin.org/get
```
