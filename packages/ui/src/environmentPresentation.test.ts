import assert from "node:assert/strict";
import test from "node:test";
import { environmentOptionLabel, LOCAL_ENVIRONMENT_NAME } from "./environmentPresentation";

test("labels the persisted local environment as the default environment", () => {
  assert.equal(
    environmentOptionLabel({ id: "local-env", name: "Local" }),
    `${LOCAL_ENVIRONMENT_NAME} \u00b7 \u9ed8\u8ba4`,
  );
});

test("keeps custom environment names and supplies an empty-name fallback", () => {
  assert.equal(environmentOptionLabel({ id: "staging", name: "\u6d4b\u8bd5" }), "\u6d4b\u8bd5");
  assert.equal(environmentOptionLabel({ id: "empty", name: "  " }), "\u672a\u547d\u540d\u73af\u5883");
});
