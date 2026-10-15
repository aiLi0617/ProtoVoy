import { spawn } from "node:child_process";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { startE2EServer } from "./e2e-server.mjs";

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const playwrightCli = join(repoRoot, "node_modules", "@playwright", "test", "cli.js");
const localProxyBypass = ["127.0.0.1", "localhost", process.env.NO_PROXY, process.env.no_proxy]
  .filter(Boolean)
  .join(",");
const hasExplicitConfig = process.argv
  .slice(2)
  .some((argument) => argument === "--config" || argument.startsWith("--config="));
const configArgs = hasExplicitConfig ? [] : ["--config=playwright.managed.config.ts"];

const args = process.argv.slice(2);
const needsServer = !args.includes("--list") && !args.includes("--help") && !args.includes("-h");
const server = needsServer ? await startE2EServer() : null;
let stopping = false;
async function stopForSignal(signal) {
  if (stopping) return;
  stopping = true;
  await server?.stop();
  process.exit(signal === "SIGINT" ? 130 : 143);
}
process.once("SIGINT", () => void stopForSignal("SIGINT"));
process.once("SIGTERM", () => void stopForSignal("SIGTERM"));

try {
  const result = await new Promise((resolveResult, reject) => {
    const child = spawn(process.execPath, [playwrightCli, "test", ...configArgs, ...args], {
      cwd: repoRoot,
      env: {
        ...process.env,
        NO_PROXY: localProxyBypass,
        no_proxy: localProxyBypass,
        PROTOVOY_E2E_PORT: String(server?.port ?? process.env.PROTOVOY_E2E_PORT ?? 5180),
        PROTOVOY_E2E_EXTERNAL_SERVER: needsServer ? "1" : process.env.PROTOVOY_E2E_EXTERNAL_SERVER,
      },
      stdio: "inherit",
      windowsHide: true,
    });
    child.once("error", reject);
    child.once("exit", (code, signal) => resolveResult({ code, signal }));
  });
  if (result.signal) throw new Error(`Playwright exited after signal ${result.signal}.`);
  process.exitCode = result.code ?? 1;
} finally {
  await server?.stop();
}
