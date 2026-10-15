import assert from "node:assert/strict";
import test from "node:test";
import { selectableEnvironmentOptions } from "./HttpWorkbench";

test("uses the independent local environment and hides the global record", () => {
  assert.deepEqual(selectableEnvironmentOptions([
    { id: "default-env", name: "Default" },
    { id: "local-env", name: "Local" },
    { id: "staging", name: "Staging" },
  ]), [
    { id: "local-env", name: "Local" },
    { id: "staging", name: "Staging" },
  ]);
});
