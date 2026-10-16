import assert from "node:assert/strict";
import test from "node:test";
import { createQueryRow, urlEncodedRows, urlWithQueryRows } from "./KeyValueEditor";

test("keeps variable templates visible while encoding query parameters", () => {
  const rows = [
    createQueryRow("{{paramName}}", "{{paramValue}}"),
    createQueryRow("search term", "hello world"),
  ];

  assert.equal(
    urlWithQueryRows("https://api.example.test/items#result", rows),
    "https://api.example.test/items?{{paramName}}={{paramValue}}&search+term=hello+world#result",
  );
});

test("keeps variable templates visible in form-urlencoded bodies", () => {
  const rows = [
    createQueryRow("account", "{{accountId}}"),
    createQueryRow("display name", "Ada Lovelace"),
  ];

  assert.equal(urlEncodedRows(rows), "account={{accountId}}&display+name=Ada+Lovelace");
});
