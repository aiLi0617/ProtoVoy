import assert from "node:assert/strict";
import { setModuleCollectionsCollapsed } from "./WorkspaceExplorer";

assert.deepEqual(
  setModuleCollectionsCollapsed(
    ["module:orders", "collection:orders-root", "request:request-1", "collection:another-module"],
    "orders",
    ["orders-root", "orders-child"],
    false,
  ),
  ["request:request-1", "collection:another-module"],
  "expanding a module should preserve request/debug-case and other-module state",
);

assert.deepEqual(
  setModuleCollectionsCollapsed(
    ["request:request-1", "collection:another-module"],
    "orders",
    ["orders-root", "orders-child"],
    true,
  ),
  ["request:request-1", "collection:another-module", "collection:orders-root", "collection:orders-child"],
  "collapsing a module should only add its collection keys",
);

console.log("WorkspaceExplorer collapse tests passed");
