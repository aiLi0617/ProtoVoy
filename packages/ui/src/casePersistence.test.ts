import assert from "node:assert/strict";
import test from "node:test";
import { sanitizeHttpRequestForPersistence, type HttpWorkbenchRequest } from "./HttpWorkbench";

test("case persistence keeps every request configuration tab", () => {
  const request: HttpWorkbenchRequest = {
    id: "case-1",
    name: "success",
    method: "POST",
    url: "https://api.example.com/users?page=2&limit=20",
    headers: [
      ["Content-Type", "application/json"],
      ["X-Tenant", "demo"],
      ["Cookie", "sid=literal-session; locale={{locale}}"],
    ],
    body: '{"name":"Ada"}',
    bodyEncoding: "text",
    timeoutMs: 12_345,
    variables: { locale: "zh-CN" },
    assertions: [],
    auth: { kind: "bearer", token: "transient-token", secret_ref: "api-token" },
    followRedirects: false,
    retryMax: 3,
    retryBackoffMs: 900,
    proxy: "http://proxy.test:8080",
    tlsVerify: false,
    tlsClientCertRef: "client-cert",
    environmentRef: "staging",
    preScripts: ["console.log('before')"],
    postScripts: ["console.log('after')"],
    metadata: { __protovoyCaseType: "debug" },
  };

  const saved = sanitizeHttpRequestForPersistence(request);

  assert.equal(saved.url, request.url, "Params are persisted in the URL");
  assert.equal(saved.body, request.body);
  assert.deepEqual(saved.headers, [
    ["Content-Type", "application/json"],
    ["X-Tenant", "demo"],
    ["Cookie", "sid=; locale={{locale}}"],
  ]);
  assert.deepEqual(saved.auth, { kind: "bearer", token: "transient-token", secret_ref: "api-token" });
  assert.deepEqual(saved.preScripts, request.preScripts);
  assert.deepEqual(saved.postScripts, request.postScripts);
  assert.equal(saved.timeoutMs, request.timeoutMs);
  assert.equal(saved.followRedirects, request.followRedirects);
  assert.equal(saved.retryMax, request.retryMax);
  assert.equal(saved.retryBackoffMs, request.retryBackoffMs);
  assert.equal(saved.proxy, request.proxy);
  assert.equal(saved.tlsVerify, request.tlsVerify);
  assert.equal(saved.tlsClientCertRef, request.tlsClientCertRef);
  assert.equal(saved.environmentRef, request.environmentRef);
  assert.equal(request.auth?.token, "transient-token", "sanitizing must not mutate editor state");
});
