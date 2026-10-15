import { createHash } from "node:crypto";
import { existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const playwrightCli = join(repoRoot, "node_modules", "@playwright", "test", "cli.js");
const snapshotPath = join(repoRoot, "node_modules", ".cache", "protovoy", "test-changed-e2e.json");
const localProxyBypass = ["127.0.0.1", "localhost", process.env.NO_PROXY, process.env.no_proxy]
  .filter(Boolean)
  .join(",");
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
    smokeTitles: [
      "HTTP target URL has an accessible label and neutral empty state",
      "HTTP parameter entry row keeps the same height as populated rows",
      "HTTP save action renders as one control with its dropdown arrow inside",
      "HTTP workbench exposes busy state semantics",
    ],
  },
  {
    name: "settings",
    sources: [/SettingsDialog/, /userPreferences/, /EnvironmentEditor/],
    specs: ["e2e/project-settings-layout.spec.ts", "e2e/web-smoke.spec.ts"],
    smokeTitles: ["keeps project settings separate from application settings", "switches theme and keeps visible keyboard focus"],
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
    smokeTitles: [
      "starts with zero tabs and the project overview",
      "applies saved appearance before the app renders",
      "turns Agent bootstrap failures into actionable guidance",
      "opens collection automation from the project rail",
      "opens the grouped protocol workspace without horizontal overflow",
      "tab overflow tools expose every tab and allow closing the final tab",
      "shows shortcuts for the current platform",
      "uses one named lifecycle tablist",
      "moves project navigation below the workspace on narrow screens",
    ],
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
    smokeTitles: [
      "gRPC response stays bottom-aligned and uses the HTTP split drag bounds",
      "non-HTTP protocol response workbenches fill tall lifecycle panels",
      "switches workbenches and preserves URL and stored state",
      "keeps capture proxy instructions separated and copyable",
      "all protocol workbenches keep semantic frame and viewport bounds",
      "all protocol workbenches keep light theme bounds",
      "broker and database workbenches keep save action compact and request content visible",
    ],
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

const knownSpecs = new Set(allSpecs);
const missingMappedSpecs = [
  ...mobileSpecs,
  ...featureRules.flatMap((rule) => rule.specs),
].filter((spec, index, specs) => !knownSpecs.has(spec) && specs.indexOf(spec) === index);
if (missingMappedSpecs.length > 0) {
  throw new Error(`E2E selector references missing specs: ${missingMappedSpecs.join(", ")}`);
}

function run(command, args, { capture = false, dryRun = false } = {}) {
  const display = [command, ...args].join(" ");
  if (dryRun) {
    console.log(`[test:e2e:changed] ${display}`);
    return "";
  }
  const result = spawnSync(command, args, {
    cwd: repoRoot,
    encoding: "utf8",
    env: {
      ...process.env,
      NO_PROXY: localProxyBypass,
      no_proxy: localProxyBypass,
    },
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
  const options = { base: undefined, dryRun: false, files: [], runnerArgs: [], sinceLastRun: false, staged: false };
  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index];
    if (argument === "--dry-run") options.dryRun = true;
    else if (argument === "--base") options.base = argv[++index];
    else if (argument === "--file") options.files.push(argv[++index]);
    else if (argument === "--since-last-run") options.sinceLastRun = true;
    else if (argument === "--staged") options.staged = true;
    else if (argument !== "--") options.runnerArgs.push(argument);
  }
  if (options.base === undefined && argv.includes("--base")) throw new Error("--base requires a git revision");
  if (options.files.includes(undefined)) throw new Error("--file requires a repository-relative path");
  const selectors = [Boolean(options.base), options.files.length > 0, options.sinceLastRun, options.staged].filter(Boolean).length;
  if (selectors > 1) throw new Error("Use only one of --base, --file, --staged, or --since-last-run");
  return options;
}

function changedFiles(options) {
  if (options.files.length > 0) return options.files;
  const tracked = options.base
    ? gitLines(["diff", "--name-only", "--diff-filter=ACDMRT", `${options.base}...HEAD`])
    : options.staged
      ? gitLines(["diff", "--cached", "--name-only", "--diff-filter=ACDMRT"])
      : gitLines(["diff", "--name-only", "--diff-filter=ACDMRT", "HEAD"]);
  const untracked = options.base || options.staged ? [] : gitLines(["ls-files", "--others", "--exclude-standard"]);
  return [...new Set([...tracked, ...untracked])];
}

function fingerprint(file) {
  const path = resolve(repoRoot, file);
  if (!existsSync(path)) return "<deleted>";
  return createHash("sha256").update(readFileSync(path)).digest("hex");
}

function readSnapshots() {
  if (!existsSync(snapshotPath)) return {};
  try {
    return JSON.parse(readFileSync(snapshotPath, "utf8"));
  } catch {
    return {};
  }
}

function snapshotKey(runnerArgs) {
  return JSON.stringify(runnerArgs);
}

function saveSnapshot(runnerArgs, files) {
  const snapshots = readSnapshots();
  snapshots[snapshotKey(runnerArgs)] = Object.fromEntries(files.map((file) => [file, fingerprint(file)]));
  mkdirSync(dirname(snapshotPath), { recursive: true });
  writeFileSync(snapshotPath, `${JSON.stringify(snapshots, null, 2)}\n`);
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
        file === "playwright.runtime.config.ts" ||
        file === "scripts/run-e2e.mjs" ||
        file === "scripts/test-changed-e2e.mjs" ||
        (file.startsWith("e2e/") && !file.endsWith(".spec.ts")) ||
        (file.endsWith(".spec.ts") && !existsSync(resolve(repoRoot, file))),
    )
  ) {
    return { allProjects: true, reasons: ["shared Playwright configuration or e2e support code changed"], specs: allSpecs };
  }

  const selected = new Set(normalized.filter((file) => /^e2e\/.*\.spec\.ts$/.test(file)));
  const smokeTitles = new Set();
  let fullSmoke = selected.has("e2e/web-smoke.spec.ts");
  const reasons = selected.size > 0 ? ["changed e2e specs"] : [];
  for (const rule of featureRules) {
    if (normalized.some((file) => rule.sources.some((pattern) => pattern.test(file)))) {
      rule.specs.forEach((spec) => selected.add(spec));
      if (rule.specs.includes("e2e/web-smoke.spec.ts")) {
        if (rule.smokeTitles) rule.smokeTitles.forEach((title) => smokeTitles.add(title));
        else fullSmoke = true;
      }
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
    fullSmoke = true;
    reasons.push("unmapped shared UI source (smoke fallback)");
  }
  const selectedSpecs = [...selected].sort();
  const exercisesMobileLayout = selectedSpecs.some((spec) => mobileSpecs.has(spec));
  return {
    allProjects: responsiveSourceChanged || exercisesMobileLayout,
    reasons: [...new Set(reasons)],
    smokeTitles: fullSmoke ? [] : [...smokeTitles],
    specs: selectedSpecs,
  };
}

const options = parseArguments(process.argv.slice(2));
const dirtyFiles = changedFiles(options);
let files = dirtyFiles;
if (options.sinceLastRun) {
  const previous = readSnapshots()[snapshotKey(options.runnerArgs)] ?? {};
  files = dirtyFiles.filter((file) => previous[file] !== fingerprint(file));
  console.log(`[test:e2e:changed] ${files.length}/${dirtyFiles.length} dirty file(s) changed since the last successful matching run.`);
}
const selection = selectSpecs(files);
if (selection.specs.length === 0) {
  console.log("[test:e2e:changed] No browser tests are required for these files.");
  if (options.sinceLastRun && !options.dryRun) saveSnapshot(options.runnerArgs, dirtyFiles);
  process.exit(0);
}

console.log(`[test:e2e:changed] Features: ${selection.reasons.join(", ")}`);
console.log(`[test:e2e:changed] Specs (${selection.specs.length}/${allSpecs.length}): ${selection.specs.join(", ")}`);
const hasExplicitProject = options.runnerArgs.some((argument) => argument === "--project" || argument.startsWith("--project="));
const hasExplicitConfig = options.runnerArgs.some((argument) => argument === "--config" || argument.startsWith("--config="));
const projectArgs = hasExplicitProject || selection.allProjects ? [] : ["--project=desktop-chromium"];
const configArgs = hasExplicitConfig ? [] : ["--config=playwright.runtime.config.ts"];
const hasExplicitGrep = options.runnerArgs.some((argument) => argument === "--grep" || argument.startsWith("--grep="));
const escapedSmokeTitles = selection.smokeTitles.map((title) => title.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"));
const grepArgs = !hasExplicitGrep && escapedSmokeTitles.length > 0
  ? [`--grep=^(?!.*web-smoke\\.spec\\.ts)|(?:${escapedSmokeTitles.join("|")})`]
  : [];
console.log(`[test:e2e:changed] Projects: ${projectArgs.length === 0 ? "desktop + mobile" : "desktop only"}`);
if (!existsSync(playwrightCli)) throw new Error(`Playwright is not installed at ${playwrightCli}`);
run(process.execPath, [playwrightCli, "test", ...selection.specs, ...projectArgs, ...configArgs, ...grepArgs, ...options.runnerArgs], options);
if (options.sinceLastRun && !options.dryRun) saveSnapshot(options.runnerArgs, dirtyFiles);
