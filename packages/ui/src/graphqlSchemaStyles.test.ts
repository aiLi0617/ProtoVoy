import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const styles = readFileSync(new URL("./styles.css", import.meta.url), "utf8");

assert.match(
  styles,
  /\.http-graphql-schema\s*\{[^}]*width:\s*max-content[^}]*flex:\s*0 0 auto/,
  "GraphQL schema action must size to its full text instead of shrinking",
);
assert.doesNotMatch(
  styles,
  /\.http-graphql-schema\s*\{[^}]*(?:text-overflow:\s*ellipsis|max-width:)/,
  "GraphQL schema action must not truncate its status text",
);

console.log("GraphQL schema style tests passed");
