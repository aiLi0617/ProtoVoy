import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const source = readFileSync(new URL("./HttpWorkbench.tsx", import.meta.url), "utf8");

assert.match(
  source,
  /activeTab === "cookies" && <div className="http-kv-editor" aria-label="请求 Cookies">/,
  "the Cookies editor should use the same gap-free key-value layout as Headers",
);

console.log("HTTP workbench layout tests passed");
