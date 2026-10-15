import assert from "node:assert/strict";
import test from "node:test";
import { bodyPayloadCount } from "./HttpWorkbench";

test("uses one Body item for every non-empty raw payload", () => {
  assert.equal(bodyPayloadCount('{"purchaseDemandId": 1, "units": []}'), 1);
  assert.equal(bodyPayloadCount('[{"id": 1}, {"id": 2}]'), 1);
  assert.equal(bodyPayloadCount('"hello"'), 1);
  assert.equal(bodyPayloadCount("<request />"), 1);
  assert.equal(bodyPayloadCount("{}"), 1);
});

test("does not show a Body count for an empty payload", () => {
  assert.equal(bodyPayloadCount("  "), 0);
});
