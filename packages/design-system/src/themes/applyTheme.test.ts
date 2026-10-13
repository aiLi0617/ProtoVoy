import assert from "node:assert/strict";
import { applyTheme, resolveTheme } from "./applyTheme";

assert.equal(resolveTheme("dark"), "dark");
assert.equal(resolveTheme("light"), "light");
assert.equal(applyTheme("dark", { dataset: {} } as unknown as HTMLElement), "dark");
console.log("applyTheme tests passed");
