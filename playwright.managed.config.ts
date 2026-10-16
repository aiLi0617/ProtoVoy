import baseConfig from "./playwright.config";

// scripts/run-e2e.mjs owns the Vite process so it can always reap the exact
// child it started, including on Windows where shell-launched grandchildren
// otherwise tend to survive the Playwright process.
export default {
  ...baseConfig,
  webServer: undefined,
};
