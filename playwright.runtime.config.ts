import baseConfig from "./playwright.config";

const e2ePort = process.env.PROTOVOY_E2E_PORT ?? "5180";

export default {
  ...baseConfig,
  webServer: {
    command: `node node_modules/vite/bin/vite.js apps/web --host 127.0.0.1 --port ${e2ePort}`,
    url: `http://127.0.0.1:${e2ePort}`,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
};
