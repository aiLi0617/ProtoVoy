import assert from "node:assert/strict";
import { shortcutLabel, shortcutModifier } from "./platform";

assert.equal(shortcutModifier("Win32"), "Ctrl");
assert.equal(shortcutModifier("Linux x86_64"), "Ctrl");
assert.equal(shortcutModifier("MacIntel"), "⌘");
assert.equal(shortcutModifier("iPhone"), "⌘");
assert.equal(shortcutLabel("K", "Win32"), "Ctrl K");
assert.equal(shortcutLabel(",", "MacIntel"), "⌘ ,");

console.log("Platform shortcut tests passed");
