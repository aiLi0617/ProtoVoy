import { existsSync, readFileSync, readdirSync } from "node:fs";
import { dirname, extname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
import ts from "typescript";

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
  const options = { dryRun: false, explain: false, files: [] };
  for (let index = 0; index < argv.length; index += 1) {
    if (argv[index] === "--dry-run") options.dryRun = true;
    else if (argv[index] === "--explain") options.explain = true;
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

function moduleSpecifierText(node) {
  return node && ts.isStringLiteralLike(node) ? node.text : undefined;
}

function collectImports(file, source) {
  const runtime = [];
  const typeOnly = [];
  const sourceFile = ts.createSourceFile(file, source, ts.ScriptTarget.Latest, false, file.endsWith(".tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS);
  function add(specifier, isTypeOnly) {
    if (specifier?.startsWith(".")) (isTypeOnly ? typeOnly : runtime).push(specifier);
  }
  function visit(node) {
    if (ts.isImportDeclaration(node)) {
      const clause = node.importClause;
      const bindings = clause?.namedBindings;
      const onlyTypeBindings = bindings && ts.isNamedImports(bindings) && bindings.elements.length > 0 && bindings.elements.every((element) => element.isTypeOnly);
      add(moduleSpecifierText(node.moduleSpecifier), Boolean(clause?.isTypeOnly || (!clause?.name && onlyTypeBindings)));
    } else if (ts.isExportDeclaration(node)) {
      const exports = node.exportClause;
      const onlyTypeExports = exports && ts.isNamedExports(exports) && exports.elements.length > 0 && exports.elements.every((element) => element.isTypeOnly);
      add(moduleSpecifierText(node.moduleSpecifier), Boolean(node.isTypeOnly || onlyTypeExports));
    } else if (ts.isCallExpression(node) && node.expression.kind === ts.SyntaxKind.ImportKeyword) {
      add(moduleSpecifierText(node.arguments[0]), false);
    } else if (ts.isImportEqualsDeclaration(node) && ts.isExternalModuleReference(node.moduleReference)) {
      add(moduleSpecifierText(node.moduleReference.expression), node.isTypeOnly);
    }
    ts.forEachChild(node, visit);
  }
  visit(sourceFile);
  return { runtime, typeOnly };
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
const selectionReasons = new Map();
if (
  normalizedChanges.some((file) =>
    [
      "pnpm-lock.yaml",
      "pnpm-workspace.yaml",
      "packages/ui/package.json",
      "scripts/run-tsx-tests.mjs",
      "scripts/test-changed-ui.mjs",
    ].includes(file),
  )
) {
  selected = tests;
  for (const test of tests) selectionReasons.set(test, "shared manifest or test runner changed");
} else {
  const imports = new Map();
  const runtimeDependencies = new Set();
  const typeOnlyDependencies = new Set();
  for (const file of sourceFiles.filter((candidate) => extname(candidate) !== ".css")) {
    const dependencies = [];
    const source = readFileSync(file, "utf8");
    const parsedImports = collectImports(file, source);
    for (const specifier of parsedImports.runtime) {
      const dependency = resolveImport(file, specifier);
      if (dependency) {
        dependencies.push(dependency);
        runtimeDependencies.add(dependency);
      }
    }
    for (const specifier of parsedImports.typeOnly) {
      const dependency = resolveImport(file, specifier);
      if (dependency) typeOnlyDependencies.add(dependency);
    }
    imports.set(file, dependencies);
  }

  function pathToChanged(file, seen = new Set()) {
    if (changedSources.has(file)) return [file];
    if (seen.has(file)) return undefined;
    seen.add(file);
    for (const dependency of imports.get(file) ?? []) {
      const path = pathToChanged(dependency, new Set(seen));
      if (path) return [file, ...path];
    }
    return undefined;
  }

  selected = tests.filter((test) => {
    const path = pathToChanged(test);
    if (!path) return false;
    selectionReasons.set(test, path.map((file) => relative(uiRoot, file).replaceAll("\\", "/")).join(" -> "));
    return true;
  });
  for (const changed of changedSources) {
    if (/\.test\.tsx?$/.test(changed) && existsSync(changed) && !selected.includes(changed)) {
      selected.push(changed);
      selectionReasons.set(changed, "changed test file");
    }
    const stem = changed.replace(/\.(tsx?|css)$/, "");
    for (const test of tests) if (test.startsWith(`${stem}.`) && !selected.includes(test)) {
      selected.push(test);
      selectionReasons.set(test, "matching colocated test");
    }
  }
  const onlyTypeDependenciesChanged = changedSources.size > 0 && [...changedSources].every(
    (changed) => typeOnlyDependencies.has(changed) && !runtimeDependencies.has(changed),
  );
  if (changedSources.size > 0 && selected.length === 0 && !onlyTypeDependenciesChanged) {
    selected = tests;
    for (const test of tests) selectionReasons.set(test, "unmapped UI source fallback");
  }
}

if (selected.length === 0) {
  console.log("[test:ui:changed] No UI unit tests are required.");
  process.exit(0);
}

const relativeTests = selected.sort().map((file) => relative(uiRoot, file).replaceAll("\\", "/"));
console.log(`[test:ui:changed] Selected ${relativeTests.length}/${tests.length}: ${relativeTests.join(", ")}`);
if (options.explain) {
  for (const test of selected.sort()) {
    console.log(`[test:ui:changed] Why ${relative(uiRoot, test).replaceAll("\\", "/")}: ${selectionReasons.get(test) ?? "selected"}`);
  }
}
const args = ["scripts/run-tsx-tests.mjs", "--cwd", "packages/ui", "--concurrency", "4", ...relativeTests];
if (options.dryRun) {
  console.log(`[test:ui:changed] node ${args.join(" ")}`);
  process.exit(0);
}
const result = spawnSync(process.execPath, args, { cwd: repoRoot, stdio: "inherit" });
if (result.error) throw result.error;
process.exit(result.status ?? 1);
