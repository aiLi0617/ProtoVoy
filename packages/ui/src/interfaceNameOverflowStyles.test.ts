import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const component = readFileSync(new URL("./InterfaceLifecycle.tsx", import.meta.url), "utf8");
const styles = readFileSync(new URL("./styles.css", import.meta.url), "utf8");

assert.match(
  component,
  /className="interface-lifecycle-tabs"[^>]*style=\{\{ flex: "0 0 auto"/,
  "interface lifecycle tabs must not be displaced by a long interface name",
);
assert.match(
  styles,
  /\.interface-lifecycle-name-slot\s*\{[^}]*min-width:\s*0[^}]*overflow:\s*hidden/,
  "interface name slot must contain long names within its allocated space",
);
assert.match(
  styles,
  /\.interface-lifecycle-name-control\s*\{[^}]*max-width:\s*100%[^}]*overflow:\s*hidden/,
  "interface name control must not paint over neighboring lifecycle tabs",
);
assert.match(
  styles,
  /\.interface-lifecycle-name-control\s*>\s*label\s*>\s*input\s*\{[^}]*max-width:\s*100%!important/,
  "interface name input must shrink to the remaining header width",
);

console.log("Interface name overflow style tests passed");
