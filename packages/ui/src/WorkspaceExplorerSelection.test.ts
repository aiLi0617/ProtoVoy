import assert from "node:assert/strict";
import { isCollectionActive } from "./WorkspaceExplorer";

assert.equal(
  isCollectionActive("orders-root", "orders-root", null),
  true,
  "a directly selected collection should be active",
);
assert.equal(
  isCollectionActive("orders-root", "orders-root", "request-1"),
  false,
  "a collection should not remain active when one of its requests is selected",
);
assert.equal(
  isCollectionActive("orders-root", "default-root", null),
  false,
  "an unrelated collection should not be active",
);

console.log("WorkspaceExplorer selection tests passed");
