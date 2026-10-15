import { spawnSync } from "node:child_process";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const playwrightCli = join(repoRoot, "node_modules", "@playwright", "test", "cli.js");
const localProxyBypass = ["127.0.0.1", "localhost", process.env.NO_PROXY, process.env.no_proxy]
  .filter(Boolean)
  .join(",");
const hasExplicitConfig = process.argv
  .slice(2)
  .some((argument) => argument === "--config" || argument.startsWith("--config="));
const configArgs = hasExplicitConfig ? [] : ["--config=playwright.runtime.config.ts"];

const result = spawnSync(process.execPath, [playwrightCli, "test", ...configArgs, ...process.argv.slice(2)], {
  cwd: repoRoot,
  env: {
    ...process.env,
    NO_PROXY: localProxyBypass,
    no_proxy: localProxyBypass,
  },
  stdio: "inherit",
});

if (result.error) throw result.error;
process.exit(result.status ?? 1);
