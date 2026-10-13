import assert from "node:assert/strict";
import test from "node:test";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { CodeEditor } from "./CodeEditor";

test("renders response content with a themed fallback while Monaco is loading", () => {
  const markup = renderToStaticMarkup(<CodeEditor value={'{\n  "ok": true\n}'} onChange={() => {}} readOnly bare/>);

  assert.match(markup, /class="code-editor code-editor-bare"/);
  assert.match(markup, /aria-label="代码编辑器正在加载"/);
  assert.match(markup, /background:var\(--protovoy-bg-elevated\)/);
  assert.match(markup, /&quot;ok&quot;: true/);
});
