import { existsSync, readdirSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const pnpmCommand = process.platform === "win32" ? "pnpm.cmd" : "pnpm";
const allSpecs = readdirSync(resolve(repoRoot, "e2e"))
  .filter((file) => file.endsWith(".spec.ts"))
  .map((file) => `e2e/${file}`)
  .sort();
const mobileSpecs = new Set([
  "e2e/http-live-request-layout.spec.ts",
  "e2e/web-smoke.spec.ts",
]);

const featureRules = [
  {
    name: "workspace explorer",
    sources: [/WorkspaceExplorer/, /workspaceTreeRows/, /apps\/web\/src\/workspaceTreeClient/],
    specs: [
      "e2e/create-directory-parent.spec.ts",
      "e2e/request-method-colors.spec.ts",
      ...allSpecs.filter((file) => file.includes("workspace-")),
    ],
  },
  {
    name: "mock",
    sources: [/Mock/, /mockGeneration/],
    specs: allSpecs.filter((file) => file.includes("mock-")),
  },
  {
    name: "response validation",
    sources: [/responseValidation/, /designedResponse/, /InterfaceLifecycle/],
    specs: [...allSpecs.filter((file) => file.includes("response-validation")), "e2e/mock-workbench.spec.ts"],
  },
  {
    name: "request history",
    sources: [/RequestHistory/, /ProtocolHistory/, /casePersistence/, /caseStructureBridge/],
    specs: allSpecs.filter((file) => file.includes("request-history-")),
  },
  {
    name: "HTTP request",
    sources: [/Http/, /KeyValueEditor/, /responseComponents/],
    specs: [
      "e2e/http-live-request-layout.spec.ts",
      "e2e/request-content-type.spec.ts",
      "e2e/response-validation.spec.ts",
    ],
  },
  {
    name: "settings",
    sources: [/SettingsDialog/, /userPreferences/, /EnvironmentEditor/],
    specs: ["e2e/project-settings-layout.spec.ts", "e2e/web-smoke.spec.ts"],
  },
  {
    name: "application shell",
    sources: [
      /AppShell/,
      /WorkbenchDeck/,
      /WorkbenchFrame/,
      /ClosableTabStrip/,
      /RovingTabList/,
      /packages\/ui\/src\/(styles|components)\.css$/,
      /apps\/(web|desktop)\/src\/App\.tsx$/,
    ],
    specs: ["e2e/project-pages.spec.ts", "e2e/web-smoke.spec.ts", "e2e/workspace-menu-geometry.spec.ts"],
  },
  {
    name: "protocol workbenches",
    sources: [
      /AiWorkbench/,
      /AmqpWorkbench/,
      /CaptureWorkbench/,
      /GraphqlWorkbench/,
      /GrpcWorkbench/,
      /KafkaWorkbench/,
      /MqttWorkbench/,
      /RedisWorkbench/,
      /RpcWorkbench/,
      /SocketWorkbench/,
      /SqlWorkbench/,
      /SseWorkbench/,
      /WebSocketWorkbench/,
    ],
    specs: ["e2e/request-history-protocols.spec.ts", "e2e/web-smoke.spec.ts"],
  },
  {
    name: "request model",
    sources: [/packages\/request-model\//],
    specs: [
      "e2e/request-history-protocols.spec.ts",
      "e2e/response-validation.spec.ts",
      "e2e/web-smoke.spec.ts",
    ],
  },
];

function run(command, args, { capture = false, dryRun = false } = {}) {
  const display = [command, ...args].join(" ");
  if (dryRun) {
    console.log(`[test:e2e:changed] ${display}`);
    return "";
  }
  const result = spawnSync(command, args, {
    cwd: repoRoot,
    encoding: "utf8",
    shell: process.platform === "win32" && command.toLowerCase().endsWith(".cmd"),
    stdio: capture ? ["ignore", "pipe", "inherit"] : "inherit",
  });
  if (result.error) throw result.error;
  if (result.status !== 0) process.exit(result.status ?? 1);
  return capture ? result.stdout.trim() : "";
}

function gitLines(args) {
  const output = run("git", args, { capture: true });
  return output ? output.split(/\r?\n/).filter(Boolean) : [];
}

function parseArguments(argv) {
  const options = { base: undefined, dryRun: false, files: [], runnerArgs: [] };
  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index];
    if (argument === "--dry-run") options.dryRun = true;
    else if (argument === "--base") options.base = argv[++index];
    else if (argument === "--file") options.files.push(argv[++index]);
    else if (argument !== "--") options.runnerArgs.push(argument);
  }
  if (options.base === undefined && argv.includes("--base")) throw new Error("--base requires a git revision");
  if (options.files.includes(undefined)) throw new Error("--file requires a repository-relative path");
  return options;
}

function changedFiles(options) {
  if (options.files.length > 0) return options.files;
  const tracked = options.base
    ? gitLines(["diff", "--name-only", "--diff-filter=ACDMRT", `${options.base}...HEAD`])
    : gitLines(["diff", "--name-only", "--diff-filter=ACDMRT", "HEAD"]);
  const untracked = options.base ? [] : gitLines(["ls-files", "--others", "--exclude-standard"]);
  return [...new Set([...tracked, ...untracked])];
}

function selectSpecs(files) {
  const normalized = files.map((file) => file.replaceAll("\\", "/"));
  const responsiveSourceChanged = normalized.some(
    (file) =>
      file.endsWith(".css") ||
      /AppShell|WorkbenchDeck|WorkbenchFrame|WorkspaceExplorer|RovingTabList/.test(file),
  );
  if (
    normalized.some(
      (file) =>
        file === "playwright.config.ts" ||
        file === "scripts/test-changed-e2e.mjs" ||
        (file.startsWith("e2e/") && !file.endsWith(".spec.ts")) ||
        (file.endsWith(".spec.ts") && !existsSync(resolve(repoRoot, file))),
    )
  ) {
    return { allProjects: true, reasons: ["shared Playwright configuration or e2e support code changed"], specs: allSpecs };
  }

  const selected = new Set(normalized.filter((file) => /^e2e\/.*\.spec\.ts$/.test(file)));
  const reasons = selected.size > 0 ? ["changed e2e specs"] : [];
  for (const rule of featureRules) {
    if (normalized.some((file) => rule.sources.some((pattern) => pattern.test(file)))) {
      rule.specs.forEach((spec) => selected.add(spec));
      reasons.push(rule.name);
    }
  }

  const hasUnmappedUiSource = normalized.some(
    (file) =>
      (/^packages\/ui\/src\/.+\.(tsx?|css)$/.test(file) || /^apps\/(web|desktop)\/src\/.+\.(tsx?|css)$/.test(file)) &&
      !featureRules.some((rule) => rule.sources.some((pattern) => pattern.test(file))),
  );
  if (hasUnmappedUiSource) {
    selected.add("e2e/web-smoke.spec.ts");
    reasons.push("unmapped shared UI source (smoke fallback)");
  }
  const selectedSpecs = [...selected].sort();
  const exercisesMobileLayout = selectedSpecs.some((spec) => mobileSpecs.has(spec));
  return {
    allProjects: responsiveSourceChanged || exercisesMobileLayout,
    reasons: [...new Set(reasons)],
    specs: selectedSpecs,
  };
}

const options = parseArguments(process.argv.slice(2));
const files = changedFiles(options);
const selection = selectSpecs(files);
if (selection.specs.length === 0) {
  console.log("[test:e2e:changed] No browser tests are required for these files.");
  process.exit(0);
}

console.log(`[test:e2e:changed] Features: ${selection.reasons.join(", ")}`);
console.log(`[test:e2e:changed] Specs (${selection.specs.length}/${allSpecs.length}): ${selection.specs.join(", ")}`);
const hasExplicitProject = options.runnerArgs.some((argument) => argument === "--project" || argument.startsWith("--project="));
const projectArgs = hasExplicitProject || selection.allProjects ? [] : ["--project=desktop-chromium"];
console.log(`[test:e2e:changed] Projects: ${projectArgs.length === 0 ? "desktop + mobile" : "desktop only"}`);
run(pnpmCommand, ["exec", "playwright", "test", ...selection.specs, ...projectArgs, ...options.runnerArgs], options);
