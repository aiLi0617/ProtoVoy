import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const primitiveStyles = readFileSync(new URL("../../design-system/src/primitives/components.css", import.meta.url), "utf8");

assert.match(primitiveStyles, /button:disabled\s*\{[^}]*opacity:\s*1\s*!important/, "disabled controls must retain readable foreground opacity");
assert.doesNotMatch(primitiveStyles, /\.ui-choice:has\(input:disabled\)[^{]*\{[^}]*opacity/, "disabled choices must use semantic colors instead of fading all content");

console.log("Disabled contrast tests passed");
