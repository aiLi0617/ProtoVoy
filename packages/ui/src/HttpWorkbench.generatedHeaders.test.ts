import assert from "node:assert/strict";
import test from "node:test";
import { generatedRequestHeaders } from "./HttpWorkbench";

test("builds the automatic HTTP header preview from the request URL and cookies", () => {
  const headers = generatedRequestHeaders("https://api.example.com:8443/users", [], "session=abc");
  assert.equal(headers.length, 8);
  assert.equal(headers.find((header) => header.name === "Host")?.value, "api.example.com:8443");
  assert.equal(headers.find((header) => header.name === "Cookie")?.value, "session=abc");
});

test("hides automatic rows replaced by enabled custom headers", () => {
  const headers = generatedRequestHeaders("https://api.example.com", [["accept", "application/json"], ["X-Test", "yes"]]);
  assert.equal(headers.some((header) => header.name === "Accept"), false);
  assert.equal(headers.length, 7);
});

test("keeps computed placeholders while a URL is incomplete", () => {
  const host = generatedRequestHeaders("api.example.com", []).find((header) => header.name === "Host");
  assert.equal(host?.value, "<发送请求时计算>");
});
