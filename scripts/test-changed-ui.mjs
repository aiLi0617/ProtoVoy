import { existsSync, readFileSync, readdirSync } from "node:fs";
import { dirname, extname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const uiRoot = join(repoRoot, "packages", "ui");
const srcRoot = join(uiRoot, "src");
const testGroups = new Map([
  ["src/Components.test.tsx", ["src/CodeEditor.test.tsx", "src/MockWorkbench.test.ts", "src/mockGeneration.test.ts"]],
]);

function walk(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    return entry.isDirectory() ? walk(path) : [path];
  });
}

function parseArguments(argv) {
  const options = { dryRun: false, files: [] };
  for (let index = 0; index < argv.length; index += 1) {
    if (argv[index] === "--dry-run") options.dryRun = true;
    else if (argv[index] === "--file") options.files.push(argv[++index]);
    else throw new Error(`Unknown argument: ${argv[index]}`);
  }
  if (options.files.includes(undefined)) throw new Error("--file requires a repository-relative path");
  return options;
}

function resolveImport(fromFile, specifier) {
  if (!specifier.startsWith(".")) return undefined;
  const base = resolve(dirname(fromFile), specifier);
  const candidates = [base, ...[".ts", ".tsx", ".css"].map((suffix) => `${base}${suffix}`), join(base, "index.ts"), join(base, "index.tsx")];
  return candidates.find((candidate) => existsSync(candidate));
}

const options = parseArguments(process.argv.slice(2));
const sourceFiles = walk(srcRoot).filter((file) => [".ts", ".tsx", ".css"].includes(extname(file)));
const packageJson = JSON.parse(readFileSync(join(uiRoot, "package.json"), "utf8"));
const configuredTestPaths = [...(packageJson.scripts?.test ?? "").matchAll(/(?:^|&&)\s*tsx\s+([^\s&]+)/g)].map((match) => match[1]);
const configuredTests = [...new Set(configuredTestPaths.flatMap((file) => [file, ...(testGroups.get(file) ?? [])]))]
  .map((file) => resolve(uiRoot, file))
  .filter((file) => existsSync(file));
const tests = configuredTests.length > 0 ? configuredTests : sourceFiles.filter((file) => /\.test\.tsx?$/.test(file));
const normalizedChanges = options.files.map((file) => file.replaceAll("\\", "/"));
const changedSources = new Set(normalizedChanges.filter((file) => file.startsWith("packages/ui/src/")).map((file) => resolve(repoRoot, file)));

let selected = [];
if (
  normalizedChanges.some((file) =>
    [
      "package.json",
      "pnpm-lock.yaml",
      "pnpm-workspace.yaml",
      "packages/ui/package.json",
      "scripts/run-tsx-tests.mjs",
      "scripts/test-changed-ui.mjs",
    ].includes(file),
  )
) {
  selected = tests;
} else {
  const imports = new Map();
  for (const file of sourceFiles.filter((candidate) => extname(candidate) !== ".css")) {
    const dependencies = [];
    for (const match of readFileSync(file, "utf8").matchAll(/(?:from\s*|import\s*)["']([^"']+)["']/g)) {
      const dependency = resolveImport(file, match[1]);
      if (dependency) dependencies.push(dependency);
    }
    imports.set(file, dependencies);
  }

  function reachesChanged(file, seen = new Set()) {
    if (changedSources.has(file)) return true;
    if (seen.has(file)) return false;
    seen.add(file);
    return (imports.get(file) ?? []).some((dependency) => reachesChanged(dependency, new Set(seen)));
  }

  selected = tests.filter((test) => reachesChanged(test));
  for (const changed of changedSources) {
    if (/\.test\.tsx?$/.test(changed) && existsSync(changed) && !selected.includes(changed)) selected.push(changed);
    const stem = changed.replace(/\.(tsx?|css)$/, "");
    for (const test of tests) if (test.startsWith(`${stem}.`) && !selected.includes(test)) selected.push(test);
  }
  if (changedSources.size > 0 && selected.length === 0) selected = tests;
}

if (selected.length === 0) {
  console.log("[test:ui:changed] No UI unit tests are required.");
  process.exit(0);
}

const relativeTests = selected.sort().map((file) => relative(uiRoot, file).replaceAll("\\", "/"));
console.log(`[test:ui:changed] Selected ${relativeTests.length}/${tests.length}: ${relativeTests.join(", ")}`);
const args = ["scripts/run-tsx-tests.mjs", "--cwd", "packages/ui", "--concurrency", "4", ...relativeTests];
if (options.dryRun) {
  console.log(`[test:ui:changed] node ${args.join(" ")}`);
  process.exit(0);
}
const result = spawnSync(process.execPath, args, { cwd: repoRoot, stdio: "inherit" });
if (result.error) throw result.error;
process.exit(result.status ?? 1);
