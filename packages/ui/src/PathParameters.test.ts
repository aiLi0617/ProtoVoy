import assert from "node:assert/strict";
import test from "node:test";
import { pathParameterNames, resolvePathParameterUrl } from "./pathParameters";

test("recognizes only single-brace variables in the URL path", () => {
  assert.deepEqual(
    pathParameterNames("http://localhost/admin/{quoteId}/items/{itemId}/{quoteId}?state={queryValue}&host={{host}}"),
    ["quoteId", "itemId"],
  );
});

test("resolves encoded path values without changing query placeholders or environment variables", () => {
  assert.equal(
    resolvePathParameterUrl(
      "https://{{host}}/quotes/{quoteId}/items/{itemId}?source={queryValue}",
      new Map([["quoteId", "quote 42"], ["itemId", "{{itemId}}"]]),
    ),
    "https://{{host}}/quotes/quote%2042/items/{{itemId}}?source={queryValue}",
  );
});
