import assert from "node:assert/strict";
import test from "node:test";
import { defaultRequestTab, type HttpWorkbenchRequest } from "./HttpWorkbench";

function request(overrides: Partial<HttpWorkbenchRequest> = {}): HttpWorkbenchRequest {
  return {
    url: "https://api.example.com/orders",
    method: "POST",
    headers: [],
    timeoutMs: 30_000,
    variables: {},
    assertions: [],
    followRedirects: true,
    retryMax: 0,
    retryBackoffMs: 250,
    tlsVerify: true,
    ...overrides,
  };
}

test("opens Params when a saved request has query parameters", () => {
  assert.equal(defaultRequestTab(request({
    url: "https://api.example.com/orders?status=pending",
    body: '{"limit":10}',
  })), "params");
});

test("opens Body when a saved request only has body content", () => {
  assert.equal(defaultRequestTab(request({ body: '{"limit":10}' })), "body");
  assert.equal(defaultRequestTab(request({ bodySource: '{"type":"graphql"}' })), "body");
  assert.equal(defaultRequestTab(request({ multipart: [{ name: "file", value: "content" }] })), "body");
});

test("falls back to Params when a saved request has neither params nor body", () => {
  assert.equal(defaultRequestTab(request()), "params");
  assert.equal(defaultRequestTab(request({ body: "   ", bodySource: "" })), "params");
});
