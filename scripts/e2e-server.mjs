import http from "node:http";
import { createHash } from "node:crypto";
import { readFileSync, realpathSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { createServer } from "vite";

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const host = "127.0.0.1";
const healthPath = "/__protovoy_e2e_health";
const serverIdentity = createHash("sha256")
  .update(realpathSync(repoRoot))
  .update(readFileSync(fileURLToPath(import.meta.url)))
  .update(readFileSync(join(repoRoot, "package.json")))
  .update(readFileSync(join(repoRoot, "pnpm-lock.yaml")))
  .update(readFileSync(join(repoRoot, "apps", "web", "vite.config.ts")))
  .update(readFileSync(join(repoRoot, "playwright.managed.config.ts")))
  .digest("hex");

function isHealthy(port) {
  return new Promise((resolveHealth) => {
    const request = http.get({ host, port, path: healthPath, timeout: 1_000 }, (response) => {
      let body = "";
      response.setEncoding("utf8");
      response.on("data", (chunk) => (body += chunk));
      response.on("end", () => resolveHealth(response.statusCode === 200 && body === serverIdentity));
    });
    request.on("timeout", () => request.destroy());
    request.on("error", () => resolveHealth(false));
  });
}

export async function startE2EServer({ port = Number(process.env.PROTOVOY_E2E_PORT ?? 5180) } = {}) {
  if (!Number.isInteger(port) || port < 1 || port > 65_535) throw new Error(`Invalid E2E port: ${port}`);
  if (await isHealthy(port)) return { owned: false, port, stop: async () => {} };

  // Hosting Vite inside the runner process avoids shell and grandchild leaks
  // on Windows. If the runner is terminated, the listening socket dies too.
  const healthPlugin = {
    name: "protovoy-e2e-health",
    configureServer(server) {
      server.middlewares.use(healthPath, (_request, response) => {
        response.statusCode = 200;
        response.setHeader("content-type", "text/plain; charset=utf-8");
        response.end(serverIdentity);
      });
    },
  };
  let vite = await createServer({ plugins: [healthPlugin], root: join(repoRoot, "apps", "web"), server: { host, port, strictPort: true } });
  try {
    await vite.listen();
  } catch (error) {
    await vite.close();
    if (!/Port \d+ is already in use/i.test(error instanceof Error ? error.message : "")) throw error;
    vite = await createServer({ plugins: [healthPlugin], root: join(repoRoot, "apps", "web"), server: { host, port: 0, strictPort: false } });
    await vite.listen();
  }
  const address = vite.httpServer?.address();
  const actualPort = typeof address === "object" && address ? address.port : port;
  vite.printUrls();

  let stopped = false;
  return {
    owned: true,
    port: actualPort,
    async stop() {
      if (stopped) return;
      stopped = true;
      await vite.close();
    },
  };
}

async function runStandalone() {
  const server = await startE2EServer();
  if (!server.owned) {
    console.log(`E2E server is already running on http://${host}:${server.port}.`);
    return;
  }
  const stop = async () => {
    await server.stop();
    process.exit(0);
  };
  process.once("SIGINT", () => void stop());
  process.once("SIGTERM", () => void stop());
  await new Promise(() => {});
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  runStandalone().catch((error) => {
    console.error(error instanceof Error ? error.message : error);
    process.exit(1);
  });
}
