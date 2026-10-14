import assert from "node:assert/strict";
import test from "node:test";
import { replaceContentTypeHeader, REQUEST_CONTENT_TYPES } from "./HttpWorkbench";

test("offers the requested content types", () => {
  assert.deepEqual(REQUEST_CONTENT_TYPES.slice(0, 8), [
    "application/atom+xml",
    "application/ecmascript",
    "application/json",
    "application/vnd.api+json",
    "application/javascript",
    "application/octet-stream",
    "application/ogg",
    "application/pdf",
  ]);
});

test("replaces Content-Type while preserving other header rows", () => {
  const rows = [
    { id: "content-type", enabled: true, key: "content-type", value: "application/json", description: "", valueType: "string" as const, typeSelected: false, required: false },
    { id: "accept", enabled: true, key: "Accept", value: "*/*", description: "", valueType: "string" as const, typeSelected: false, required: false },
  ];
  const updated = replaceContentTypeHeader(rows, "application/vnd.api+json");
  assert.equal(updated.filter((row) => row.key.toLowerCase() === "content-type").length, 1);
  assert.equal(updated[0].value, "application/vnd.api+json");
  assert.equal(updated[1].key, "Accept");
});
