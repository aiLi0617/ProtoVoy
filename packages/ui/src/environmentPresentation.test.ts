import assert from "node:assert/strict";
import test from "node:test";
import { environmentOptionLabel, LOCAL_ENVIRONMENT_NAME } from "./environmentPresentation";

test("labels the persisted default environment as the local environment", () => {
  assert.equal(
    environmentOptionLabel({ id: "default-env", name: "Default" }),
    `${LOCAL_ENVIRONMENT_NAME} · 默认`,
  );
});

test("keeps custom environment names and supplies an empty-name fallback", () => {
  assert.equal(environmentOptionLabel({ id: "staging", name: "测试" }), "测试");
  assert.equal(environmentOptionLabel({ id: "empty", name: "  " }), "未命名环境");
});
