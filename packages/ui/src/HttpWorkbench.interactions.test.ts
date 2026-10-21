import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const httpSource = readFileSync(new URL("./HttpWorkbench.tsx", import.meta.url), "utf8");
const splitPaneSource = readFileSync(new URL("./WorkbenchFrame.tsx", import.meta.url), "utf8");

test("renders inherited Header parameters after request-local rows", () => {
  const headerRowsIndex = httpSource.indexOf("<KeyValueRows rows={headerRows}");
  const globalParametersIndex = httpSource.indexOf('<section className="http-global-parameters"');
  assert.ok(headerRowsIndex >= 0 && globalParametersIndex > headerRowsIndex);
});

test("refreshes environment options after an environment is saved", () => {
  assert.match(httpSource, /window\.addEventListener\("protovoy-environment-updated", load\)/);
});

test("lets the JSON message editor fill the remaining request pane height", () => {
  assert.match(httpSource, /http-request-interaction-content\$\{requestTab === "body" && bodyMode === "json" \? " is-body" : ""\}/);
  assert.match(httpSource, /className="http-body-panel"/);
  assert.match(httpSource, /className="http-body-json-editor"[\s\S]*height="100%"/);
});

test("keeps global Header parameters compact", () => {
  assert.doesNotMatch(httpSource, /继承环境配置；此处开关仅影响当前请求/);
  assert.doesNotMatch(httpSource, /aria-hidden="true">=<\/span>/);
});

test("renders request failures as an actionable response state", () => {
  assert.match(httpSource, /className="http-response-error" role="alert"/);
  assert.match(httpSource, /responseErrorGuidance\(result\.error\)/);
  assert.match(httpSource, /复制错误信息/);
});

test("uses the full response row and shared double-chevron icons for collapsing", () => {
  assert.match(splitPaneSource, /className="split-response-heading"[\s\S]*onClick=.*toggleSecondary/);
  assert.match(splitPaneSource, /"expand-all" : "collapse-all"/);
});
