import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

test("hydrates pending workbench requests before the browser paints", () => {
  const hydrationSource = readFileSync(new URL("./useWorkbenchHydration.ts", import.meta.url), "utf8");
  assert.match(hydrationSource, /useLayoutEffect\(\(\) => \{[\s\S]*consumeHydrate/);
});

test("applies external HTTP requests before the browser paints", () => {
  const workbenchSource = readFileSync(new URL("./HttpWorkbench.tsx", import.meta.url), "utf8");
  assert.match(workbenchSource, /useLayoutEffect\(\(\) => \{\s*if \(externalRequest\)/);
});
