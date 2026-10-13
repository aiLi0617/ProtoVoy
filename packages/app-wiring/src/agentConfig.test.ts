import assert from "node:assert/strict";
import { resolveAgentUrl } from "./index";

assert.equal(resolveAgentUrl({ envUrl: "http://example.test" }), "http://example.test");
console.log("agentConfig tests passed");
