import assert from "node:assert/strict";
import test from "node:test";
import { disabledGeneratedRequestHeaders, generatedRequestHeaders } from "./HttpWorkbench";

test("locks transport-owned generated headers and leaves the others configurable", () => {
  const headers = generatedRequestHeaders("https://api.example.com", []);
  assert.equal(headers.find((header) => header.name === "Cache-Control")?.locked, true);
  assert.equal(headers.find((header) => header.name === "Content-Length")?.locked, true);
  assert.equal(headers.find((header) => header.name === "Accept")?.locked, false);
});

test("restores optional generated header selections from request metadata", () => {
  assert.deepEqual(
    [...disabledGeneratedRequestHeaders({ __protovoyDisabledGeneratedHeaders: ["Accept", "COOKIE", 42] })],
    ["accept", "cookie"],
  );
});
