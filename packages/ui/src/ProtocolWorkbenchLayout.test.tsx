import assert from "node:assert/strict";
import test from "node:test";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { ProtocolWorkbenchLayout } from "./ProtocolWorkbenchLayout";

test("renders request progress and cancellation controls while loading", () => {
  const markup = renderToStaticMarkup(
    <ProtocolWorkbenchLayout
      id="test"
      protocol="TEST"
      name="Loading request"
      target="https://example.test"
      targetLabel="Target"
      actionLabel="Send"
      loading
      result={null}
      responseTitle="Response"
      emptyResponse="No response"
      onNameChange={() => undefined}
      onTargetChange={() => undefined}
      onRun={() => undefined}
      onCancel={() => undefined}
    >
      <div>Request fields</div>
    </ProtocolWorkbenchLayout>,
  );

  assert.match(markup, /class="protocol-workbench-layout" aria-busy="true"/);
  assert.match(markup, /<button[^>]*disabled=""[^>]*aria-busy="true"[^>]*class="ui-button primary protocol-run-button"/);
  assert.match(markup, /class="ui-button-spinner" aria-hidden="true"/);
  assert.match(markup, /class="request-interaction-content protocol-request-interaction-content" inert=""/);
  assert.match(markup, /class="request-loading-overlay" role="status" aria-live="polite" aria-label="正在发送请求"/);
  assert.match(markup, /请稍候，完成后自动恢复编辑/);
  assert.match(markup, /class="ui-button danger protocol-cancel-button"/);
});
