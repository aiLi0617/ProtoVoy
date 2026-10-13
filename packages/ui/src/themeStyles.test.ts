import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const styles = readFileSync(new URL("./styles.css", import.meta.url), "utf8");
const tokens = readFileSync(
  new URL("../../design-system/src/tokens/tokens.css", import.meta.url),
  "utf8",
);

assert.match(
  tokens,
  /:root\[data-theme="light"\][\s\S]*?--protovoy-project-rail:\s*var\(--protovoy-nav\)/,
  "light project rail must use the navigation surface without a dark mix",
);
assert.match(
  styles,
  /\.project-module-nav\s*\{[^}]*background:\s*var\(--protovoy-project-rail/,
  "project rail must use its theme-aware surface token",
);
assert.match(styles, /@import\s+"@protovoy\/design-system\/styles\.css"/);

console.log("Theme style tests passed");
