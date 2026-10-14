import { spawn } from "node:child_process";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const tsxCommand = join(repoRoot, "node_modules", ".bin", process.platform === "win32" ? "tsx.cmd" : "tsx");
const testGroups = new Map([
  ["src/Components.test.tsx", ["src/CodeEditor.test.tsx", "src/MockWorkbench.test.ts", "src/mockGeneration.test.ts"]],
]);

function expandTestGroups(files) {
  return [...new Set(files.flatMap((file) => [file, ...(testGroups.get(file.replaceAll("\\", "/")) ?? [])]))];
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

function runTest(file, cwd) {
  return new Promise((resolveTest) => {
    const startedAt = performance.now();
    const child = spawn(tsxCommand, [file, "--split-component-tests"], {
      cwd,
      env: process.env,
      shell: process.platform === "win32",
    });
    let stdout = "";
    let stderr = "";
    child.stdout.on("data", (chunk) => (stdout += chunk));
    child.stderr.on("data", (chunk) => (stderr += chunk));
    child.on("error", (error) => resolveTest({ durationMs: performance.now() - startedAt, error, file, status: 1, stderr, stdout }));
    child.on("close", (status) => resolveTest({ durationMs: performance.now() - startedAt, file, status: status ?? 1, stderr, stdout }));
  });
}

const options = parseArguments(process.argv.slice(2));
if (!existsSync(tsxCommand)) throw new Error(`tsx is not installed at ${tsxCommand}`);
const packageJsonPath = join(options.cwd, "package.json");
const configuredTests = existsSync(packageJsonPath)
  ? [...(JSON.parse(readFileSync(packageJsonPath, "utf8")).scripts?.test ?? "").matchAll(/(?:^|&&)\s*tsx\s+([^\s&]+)/g)].map((match) => match[1])
  : [];
const files = (
  options.files.length > 0
    ? options.files
    : expandTestGroups(
        configuredTests.length > 0
          ? configuredTests
          : collectTests(join(options.cwd, "src")).map((file) => relative(options.cwd, file)),
      )
).sort();
if (files.some((file) => !/^[a-zA-Z0-9_./\\-]+$/.test(file))) {
  throw new Error("Test paths may contain only letters, numbers, dots, slashes, underscores, and hyphens");
}
if (files.length === 0) {
  console.log("[tsx-tests] No tests selected.");
  process.exit(0);
}

console.log(`[tsx-tests] Running ${files.length} test file(s) with concurrency ${Math.min(options.concurrency, files.length)} (${options.failFast ? "fail-fast" : "collect-all"}).`);
const suiteStartedAt = performance.now();
let nextIndex = 0;
let failed = false;
async function worker() {
  while (nextIndex < files.length && !(options.failFast && failed)) {
    const file = files[nextIndex++];
    const result = await runTest(file, options.cwd);
    const label = result.status === 0 ? "PASS" : "FAIL";
    console.log(`\n[tsx-tests] ${label} ${file} (${(result.durationMs / 1000).toFixed(2)}s)`);
    if (result.stdout) process.stdout.write(result.stdout);
    if (result.stderr) process.stderr.write(result.stderr);
    if (result.error) console.error(result.error);
    if (result.status !== 0) failed = true;
  }
}

await Promise.all(Array.from({ length: Math.min(options.concurrency, files.length) }, () => worker()));
console.log(`\n[tsx-tests] Completed in ${((performance.now() - suiteStartedAt) / 1000).toFixed(2)}s.`);
if (failed) process.exit(1);
