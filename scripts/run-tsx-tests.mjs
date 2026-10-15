import { spawn } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { dirname, join, normalize, relative, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const cssLoader = join(repoRoot, "scripts", "register-test-css.mjs");
const tsxImport = import.meta.resolve("tsx");
const durationCachePath = join(repoRoot, "node_modules", ".cache", "protovoy", "tsx-test-durations.json");
function readDurations() {
  if (!existsSync(durationCachePath)) return {};
  try {
    return JSON.parse(readFileSync(durationCachePath, "utf8"));
  } catch {
    return {};
  }
}

function writeDurations(durations) {
  mkdirSync(dirname(durationCachePath), { recursive: true });
  writeFileSync(durationCachePath, `${JSON.stringify(durations, null, 2)}\n`);
}

function collectTests(directory) {
  const tests = [];
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) tests.push(...collectTests(path));
    else if (/\.test\.tsx?$/.test(entry.name)) tests.push(path);
  }
  return tests;
}

function parseArguments(argv) {
  const options = { concurrency: 4, cwd: process.cwd(), failFast: !process.env.CI, files: [] };
  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index];
    if (argument === "--concurrency") options.concurrency = Number(argv[++index]);
    else if (argument === "--cwd") options.cwd = resolve(repoRoot, argv[++index]);
    else if (argument === "--fail-fast") options.failFast = true;
    else if (argument === "--no-fail-fast") options.failFast = false;
    else options.files.push(argument);
  }
  if (!Number.isInteger(options.concurrency) || options.concurrency < 1) {
    throw new Error("--concurrency must be a positive integer");
  }
  return options;
}

function runShard(files, cwd) {
  return new Promise((resolveTest) => {
    const startedAt = performance.now();
    const child = spawn(process.execPath, [
      `--import=${pathToFileURL(cssLoader).href}`,
      `--import=${tsxImport}`,
      "--test",
      "--test-force-exit",
      "--experimental-test-isolation=none",
      "--test-concurrency=1",
      ...files,
    ], {
      cwd,
      env: process.env,
      windowsHide: true,
    });
    let stdout = "";
    let stderr = "";
    child.stdout.on("data", (chunk) => (stdout += chunk));
    child.stderr.on("data", (chunk) => (stderr += chunk));
    child.on("error", (error) => resolveTest({ durationMs: performance.now() - startedAt, error, files, status: 1, stderr, stdout }));
    child.on("close", (status) => {
      const timings = {};
      stdout = stdout.replace(/^__PROTOVOY_TEST_TIMING__(.+)\r?\n?/gm, (_line, json) => {
        try {
          const timing = JSON.parse(json);
          timings[timing.file] = timing.importMs;
        } catch {
          // Keep malformed diagnostics visible instead of hiding them.
          return _line;
        }
        return "";
      });
      resolveTest({ durationMs: performance.now() - startedAt, files, status: status ?? 1, stderr, stdout, timings });
    });
  });
}

function buildShards(files, concurrency, durations) {
  const shards = Array.from({ length: Math.min(concurrency, files.length) }, () => ({ estimatedMs: 0, files: [] }));
  for (const file of files) {
    const shard = shards.reduce((best, candidate) => candidate.estimatedMs < best.estimatedMs ? candidate : best);
    shard.files.push(file);
    shard.estimatedMs += durations[file] ?? 1_000;
  }
  return shards;
}

const options = parseArguments(process.argv.slice(2));
const durations = readDurations();
const files = (
  options.files.length > 0
    ? options.files.map((file) => normalize(file))
    : collectTests(join(options.cwd, "src")).map((file) => relative(options.cwd, file))
).sort((left, right) => (durations[right] ?? 0) - (durations[left] ?? 0) || left.localeCompare(right));
if (files.some((file) => !/^[a-zA-Z0-9_./\\-]+$/.test(file))) {
  throw new Error("Test paths may contain only letters, numbers, dots, slashes, underscores, and hyphens");
}
if (files.length === 0) {
  console.log("[tsx-tests] No tests selected.");
  process.exit(0);
}

const shards = buildShards(files, options.concurrency, durations);
console.log(`[tsx-tests] Running ${files.length} test file(s) in ${shards.length} isolated shard process(es) (${options.failFast ? "fail-fast" : "collect-all"}).`);
const suiteStartedAt = performance.now();
let nextIndex = 0;
let failed = false;
async function worker() {
  while (nextIndex < shards.length && !(options.failFast && failed)) {
    const shard = shards[nextIndex++];
    console.log(`[tsx-tests] START shard [${shard.files.join(", ")}]`);
    const result = await runShard(shard.files, options.cwd);
    const measuredImportMs = Object.values(result.timings ?? {}).reduce((total, value) => total + value, 0);
    const sharedOverheadMs = Math.max(0, result.durationMs - measuredImportMs);
    const historicalWeight = shard.files.reduce((total, file) => total + (durations[file] ?? 1_000), 0);
    for (const file of shard.files) {
      const weightedOverhead = sharedOverheadMs * ((durations[file] ?? 1_000) / historicalWeight);
      const observedMs = Math.round((result.timings?.[file] ?? 0) + weightedOverhead);
      durations[file] = durations[file] ? Math.round((durations[file] * 3 + observedMs) / 4) : observedMs;
    }
    const label = result.status === 0 ? "PASS" : "FAIL";
    console.log(`\n[tsx-tests] ${label} shard [${shard.files.join(", ")}] (${(result.durationMs / 1000).toFixed(2)}s)`);
    if (result.stdout) process.stdout.write(result.stdout);
    if (result.stderr) process.stderr.write(result.stderr);
    if (result.error) console.error(result.error);
    if (result.status !== 0) failed = true;
  }
}

await Promise.all(Array.from({ length: shards.length }, () => worker()));
writeDurations(durations);
console.log(`\n[tsx-tests] Completed in ${((performance.now() - suiteStartedAt) / 1000).toFixed(2)}s.`);
if (failed) process.exit(1);
