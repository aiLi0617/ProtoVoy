import assert from "node:assert/strict";
import test from "node:test";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { CodeEditor } from "./CodeEditor";

test("renders response content with stable line numbers and highlighting while Monaco is loading", () => {
  const markup = renderToStaticMarkup(<CodeEditor value={'{\n  "ok": true\n}'} onChange={() => {}} language="json" readOnly bare/>);

  assert.match(markup, /class="code-editor code-editor-bare"/);
  assert.match(markup, /aria-label="代码编辑器正在加载"/);
  assert.match(markup, /background:var\(--protovoy-bg-elevated\)/);
  assert.match(markup, /class="code-editor-fallback-line-number"/);
  assert.match(markup, />2<\/span>/);
  assert.match(markup, /color:#9CDCFE/);
  assert.match(markup, /&quot;ok&quot;/);
  assert.match(markup, /color:#C586C0[^>]*>true/);
  assert.doesNotMatch(markup, /visibility:hidden/);
});
