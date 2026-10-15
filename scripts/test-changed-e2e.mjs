import { createHash } from "node:crypto";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { spawn, spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { smokeTags, validateSmokeTags } from "./e2e-smoke-tags.mjs";
import { pruneValidationCache, readValidationCache, updateValidationCache } from "./validation-cache.mjs";

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const e2eRunner = join(repoRoot, "scripts", "run-e2e.mjs");
pruneValidationCache();
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
    smokeTags: ["@http"],
  },
  {
    name: "settings",
    sources: [/SettingsDialog/, /userPreferences/, /EnvironmentEditor/],
    specs: ["e2e/project-settings-layout.spec.ts", "e2e/web-smoke.spec.ts"],
    smokeTags: ["@settings"],
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
    smokeTags: ["@app-shell"],
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
    smokeTags: ["@protocol-workbenches"],
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
const smokeSource = readFileSync(resolve(repoRoot, "e2e/web-smoke.spec.ts"), "utf8");
const missingNativeTags = [...smokeTags].filter((tag) => !smokeSource.includes(`tag: "${tag}"`));
if (missingNativeTags.length > 0) throw new Error(`E2E smoke spec is missing native tags: ${missingNativeTags.join(", ")}`);
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
  const options = { base: undefined, cache: true, dryRun: false, files: [], runnerArgs: [], sinceLastRun: false, staged: false };
  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index];
    if (argument === "--dry-run") options.dryRun = true;
    else if (argument === "--no-cache") options.cache = false;
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
  if (options.base) return gitLines(["diff", "--name-only", "--diff-filter=ACDMRT", `${options.base}...HEAD`]);
  if (options.staged) return gitLines(["diff", "--cached", "--name-only", "--diff-filter=ACDMRT"]);
  const result = spawnSync("git", ["status", "--porcelain=v1", "-z", "--untracked-files=all"], {
    cwd: repoRoot,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "ignore"],
  });
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error("Unable to inspect the Git working tree.");
  const records = result.stdout.split("\0").filter(Boolean);
  const files = [];
  for (let index = 0; index < records.length; index += 1) {
    const record = records[index];
    files.push(record.slice(3).replaceAll("\\", "/"));
    if (/[RC]/.test(record.slice(0, 2))) index += 1;
  }
  return [...new Set(files)];
}

function fingerprint(file) {
  const path = resolve(repoRoot, file);
  if (!existsSync(path)) return "<deleted>";
  return createHash("sha256").update(readFileSync(path)).digest("hex");
}

function readSnapshots() {
  return readValidationCache("test-changed-e2e");
}

function snapshotKey(runnerArgs) {
  return JSON.stringify(runnerArgs);
}

function saveSnapshot(runnerArgs, files) {
  updateValidationCache("test-changed-e2e", (snapshots) => ({
    ...snapshots,
    [snapshotKey(runnerArgs)]: Object.fromEntries(files.map((file) => [file, fingerprint(file)])),
  }));
}

function updateFailureState(key, value) {
  updateValidationCache("validation-failures", (states) => {
    const updated = { ...states };
    if (value) updated[key] = value;
    else delete updated[key];
    return updated;
  });
}

function collectFiles(directory, result = []) {
  if (!existsSync(directory)) return result;
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    if (["node_modules", "dist", "target", "test-results", "playwright-report"].includes(entry.name)) continue;
    const path = join(directory, entry.name);
    if (entry.isDirectory()) collectFiles(path, result);
    else if (entry.isFile() && !/\.(?:tsbuildinfo|log)$/.test(entry.name)) result.push(path);
  }
  return result;
}

function relativeSource(file) {
  return file.slice(repoRoot.length + 1).replaceAll("\\", "/");
}

function localImportClosure(initialFiles) {
  const closure = new Set();
  function visit(file) {
    if (closure.has(file) || !existsSync(file)) return;
    closure.add(file);
    if (file.endsWith(".css")) {
      const source = readFileSync(file, "utf8");
      for (const match of source.matchAll(/@import\s+["'](\.[^"']+)["']/g)) {
        const dependency = resolve(dirname(file), match[1]);
        if (existsSync(dependency)) visit(dependency);
      }
      return;
    }
    if (!/\.[cm]?[jt]sx?$/.test(file)) return;
    const source = readFileSync(file, "utf8");
    for (const match of source.matchAll(/(?:from\s*|import\s*(?:\(\s*)?)["'](\.[^"']+)["']/g)) {
      const base = resolve(dirname(file), match[1]);
      const dependency = [base, `${base}.ts`, `${base}.tsx`, `${base}.js`, `${base}.jsx`, `${base}.css`, join(base, "index.ts"), join(base, "index.tsx")]
        .find((candidate) => existsSync(candidate));
      if (dependency) visit(dependency);
    }
  }
  initialFiles.forEach(visit);
  return [...closure];
}

function e2eInputHash(selection, runnerArgs, changedFiles) {
  const uiAndWebSources = [
    ...collectFiles(join(repoRoot, "apps", "web")),
    ...collectFiles(join(repoRoot, "packages", "ui")),
  ];
  const selectedRules = featureRules.filter((rule) => selection.matchedRuleNames.includes(rule.name));
  const featureSources = selection.fullSourceFallback
    ? uiAndWebSources
    : uiAndWebSources.filter((file) => {
        const normalized = relativeSource(file);
        return selectedRules.some((rule) => rule.sources.some((pattern) => pattern.test(normalized)));
      });
  const selectedChangedFiles = changedFiles.filter((file) => {
    const normalized = file.replaceAll("\\", "/");
    if (selection.specs.includes(normalized)) return true;
    if (selection.fullSourceFallback) {
      return /^(?:apps\/web|packages\/(?:ui|request-model))\//.test(normalized);
    }
    return selectedRules.some((rule) => rule.sources.some((pattern) => pattern.test(normalized)));
  });
  const files = [
    ...localImportClosure(featureSources),
    ...(selection.fullSourceFallback || selection.matchedRuleNames.includes("request model")
      ? collectFiles(join(repoRoot, "packages", "request-model"))
      : []),
    ...selectedChangedFiles.map((file) => resolve(repoRoot, file)),
    ...selection.specs.map((spec) => resolve(repoRoot, spec)),
    ...[
      "package.json",
      "pnpm-lock.yaml",
      "apps/web/vite.config.ts",
      "playwright.config.ts",
      "playwright.managed.config.ts",
      "scripts/e2e-server.mjs",
      "scripts/e2e-smoke-tags.mjs",
      "scripts/run-e2e.mjs",
      "scripts/test-changed-e2e.mjs",
      "scripts/validation-cache.mjs",
    ].map((file) => resolve(repoRoot, file)),
  ];
  const hash = createHash("sha256").update(`${process.version}\0${process.platform}\0${process.arch}\0${process.env.CI ?? ""}\0${JSON.stringify(runnerArgs)}\0`);
  for (const file of [...new Set(files)].sort()) {
    hash.update(file);
    hash.update(existsSync(file) ? readFileSync(file) : "<missing>");
  }
  return hash.digest("hex");
}

function changedAdditions(options) {
  const args = options.staged
    ? ["diff", "--cached", "--unified=0"]
    : options.base
      ? ["diff", "--unified=0", `${options.base}...HEAD`]
      : ["diff", "--unified=0", "HEAD"];
  const result = spawnSync("git", args, { cwd: repoRoot, encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] });
  if (result.status !== 0) return new Map();
  const additions = new Map();
  let current;
  for (const line of result.stdout.split(/\r?\n/)) {
    if (line.startsWith("+++ ")) {
      const path = line.slice(4);
      current = path === "/dev/null" ? undefined : path.replace(/^b\//, "").replaceAll("\\", "/");
      if (current && !additions.has(current)) additions.set(current, []);
    } else if (current && line.startsWith("+") && !line.startsWith("+++")) {
      additions.get(current).push(line.slice(1));
    }
  }
  return new Map([...additions].map(([file, lines]) => [file, lines.join("\n")]));
}

function cssAffectsResponsiveLayout(file, additions) {
  if (!file.endsWith(".css")) return false;
  const path = resolve(repoRoot, file);
  const changed = additions.has(file) ? additions.get(file) : existsSync(path) ? readFileSync(path, "utf8") : "";
  return /@media|(?:min|max)-width|container-type|@container|mobile|narrow|project-module-nav|explorer-edge|workbench-tabs|tab-overflow|command-trigger/i.test(changed);
}

function selectSpecs(files, additions) {
  const normalized = files.map((file) => file.replaceAll("\\", "/"));
  const responsiveSourceChanged = normalized.some(
    (file) =>
      cssAffectsResponsiveLayout(file, additions) ||
      /AppShell|WorkbenchDeck|WorkbenchFrame|WorkspaceExplorer|RovingTabList/.test(file),
  );
  if (
    normalized.some(
      (file) =>
        file === "playwright.config.ts" ||
        file === "playwright.managed.config.ts" ||
        file === "playwright.runtime.config.ts" ||
        file === "package.json" ||
        file === "pnpm-lock.yaml" ||
        file === "apps/web/vite.config.ts" ||
        file === "scripts/e2e-server.mjs" ||
        file === "scripts/e2e-smoke-tags.mjs" ||
        file === "scripts/run-e2e.mjs" ||
        file === "scripts/test-changed-e2e.mjs" ||
        (file.startsWith("e2e/") && !file.endsWith(".spec.ts")) ||
        (file.endsWith(".spec.ts") && !existsSync(resolve(repoRoot, file))),
    )
  ) {
    return { allProjects: true, fullSourceFallback: true, matchedRuleNames: [], reasons: ["shared Playwright configuration or e2e support code changed"], smokeTags: [], specs: allSpecs };
  }

  const selected = new Set(normalized.filter((file) => /^e2e\/.*\.spec\.ts$/.test(file)));
  const selectedSmokeTags = new Set();
  const matchedRuleNames = [];
  let fullSmoke = selected.has("e2e/web-smoke.spec.ts");
  const reasons = selected.size > 0 ? ["changed e2e specs"] : [];
  for (const rule of featureRules) {
    if (normalized.some((file) => rule.sources.some((pattern) => pattern.test(file)))) {
      rule.specs.forEach((spec) => selected.add(spec));
      if (rule.specs.includes("e2e/web-smoke.spec.ts")) {
        if (rule.smokeTags) rule.smokeTags.forEach((tag) => selectedSmokeTags.add(tag));
        else fullSmoke = true;
      }
      reasons.push(rule.name);
      matchedRuleNames.push(rule.name);
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
  const explicitlyChangedMobileSpec = normalized.some((file) => mobileSpecs.has(file));
  return {
    allProjects: responsiveSourceChanged || explicitlyChangedMobileSpec,
    fullSourceFallback: hasUnmappedUiSource,
    matchedRuleNames,
    reasons: [...new Set(reasons)],
    smokeTags: fullSmoke ? [] : [...selectedSmokeTags],
    specs: selectedSpecs,
  };
}

const options = parseArguments(process.argv.slice(2));
const dirtyFiles = changedFiles(options);
const additions = changedAdditions(options);
let files = dirtyFiles;
if (options.sinceLastRun) {
  const previous = readSnapshots()[snapshotKey(options.runnerArgs)] ?? {};
  files = dirtyFiles.filter((file) => previous[file] !== fingerprint(file));
  console.log(`[test:e2e:changed] ${files.length}/${dirtyFiles.length} dirty file(s) changed since the last successful matching run.`);
}
const selection = selectSpecs(files, additions);
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
const configArgs = hasExplicitConfig ? [] : [];
const hasExplicitGrep = options.runnerArgs.some((argument) => argument === "--grep" || argument.startsWith("--grep="));
const selectedSmokeTags = validateSmokeTags(selection.smokeTags);
if (selection.smokeTags.length > 0) console.log(`[test:e2e:changed] Smoke tags: ${selection.smokeTags.join(", ")}`);
const escapedSmokeTags = selectedSmokeTags.map((tag) => tag.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"));
const grepArgs = !hasExplicitGrep && escapedSmokeTags.length > 0
  ? [`--grep=^(?!.*web-smoke\\.spec\\.ts)|(?:${escapedSmokeTags.join("|")})`]
  : [];
console.log(`[test:e2e:changed] Projects: ${projectArgs.length === 0 ? "desktop + mobile" : "desktop only"}`);
if (!existsSync(e2eRunner)) throw new Error(`E2E runner is not installed at ${e2eRunner}`);
const cacheKey = JSON.stringify({ grepArgs, projectArgs, runnerArgs: options.runnerArgs, specs: selection.specs });
const inputHash = e2eInputHash(selection, [...projectArgs, ...grepArgs, ...options.runnerArgs], files);
const e2eTaskCache = readValidationCache("e2e-task-cache");
const e2eFailureKey = `e2e:${snapshotKey(options.runnerArgs)}:${options.base ?? (options.staged ? "staged" : options.sinceLastRun ? "since" : options.files.length > 0 ? options.files.sort().join(",") : "dirty")}`;
const failureState = readValidationCache("validation-failures")[e2eFailureKey];
let playwrightHasFailedTests = false;
try {
  const lastRun = JSON.parse(readFileSync(join(repoRoot, "test-results", ".last-run.json"), "utf8"));
  playwrightHasFailedTests = Array.isArray(lastRun.failedTests) && lastRun.failedTests.length > 0;
} catch {
  // A missing or incomplete Playwright state must fall back to the full selection.
}
const retryLastFailed = options.cache && !process.env.CI && playwrightHasFailedTests && failureState?.inputHash === inputHash && !options.runnerArgs.includes("--last-failed");
const retryArgs = retryLastFailed ? ["--last-failed"] : [];
const e2eArgs = [e2eRunner, ...selection.specs, ...projectArgs, ...configArgs, ...grepArgs, ...retryArgs, ...options.runnerArgs];
const cacheableRun = !options.runnerArgs.some((argument) => /^(?:--ui|--debug|--headed|--update-snapshots|--list)(?:=|$)/.test(argument));
if (options.cache && cacheableRun && e2eTaskCache[cacheKey] === inputHash && !retryLastFailed) {
  console.log("[test:e2e:changed] CACHED selected browser tests");
  if (options.sinceLastRun && !options.dryRun) saveSnapshot(options.runnerArgs, dirtyFiles);
  process.exit(0);
}
if (retryLastFailed) console.log("[test:e2e:changed] Retrying only Playwright's last failed tests.");
if (options.dryRun) run(process.execPath, e2eArgs, options);
else {
  const result = await new Promise((resolveResult, reject) => {
    const child = spawn(process.execPath, e2eArgs, {
      cwd: repoRoot,
      env: { ...process.env, NO_PROXY: localProxyBypass, no_proxy: localProxyBypass },
      stdio: "inherit",
      windowsHide: true,
    });
    child.once("error", reject);
    child.once("exit", (status, signal) => resolveResult({ signal, status }));
  });
  if (result.signal) throw new Error(`E2E runner exited after signal ${result.signal}.`);
  if (result.status !== 0) {
    updateFailureState(e2eFailureKey, {
      files: Object.fromEntries(files.map((file) => [file, fingerprint(file)])),
      inputHash,
      pendingLabels: [`playwright -> ${selection.specs.join(",")}`],
    });
    process.exit(result.status ?? 1);
  }
  updateFailureState(e2eFailureKey);
  if (cacheableRun) updateValidationCache("e2e-task-cache", (cache) => ({ ...cache, [cacheKey]: inputHash }));
}
if (options.sinceLastRun && !options.dryRun) saveSnapshot(options.runnerArgs, dirtyFiles);
