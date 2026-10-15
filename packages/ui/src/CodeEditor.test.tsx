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

test("renders editable JSON with syntax colors instead of a gray plaintext flash", () => {
  const markup = renderToStaticMarkup(<CodeEditor value={'{\n  "count": 12\n}'} onChange={() => {}} language="json"/>);

  assert.match(markup, /class="code-editor-fallback"/);
  assert.match(markup, /aria-hidden="true"/);
  assert.match(markup, /class="code-editor-fallback-line-number"/);
  assert.match(markup, /color:#9CDCFE/);
  assert.match(markup, /color:#B5CEA8[^>]*>12/);
  assert.match(markup, /class="code-editor-fallback-input"/);
  assert.match(markup, /color:transparent/);
  assert.match(markup, /-webkit-text-fill-color:transparent/);
  assert.doesNotMatch(markup, /color:var\(--protovoy-text\)/);
});
