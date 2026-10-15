import { createHash } from "node:crypto";
import { existsSync, readFileSync, readdirSync } from "node:fs";
import { dirname, extname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { pruneValidationCache, readValidationCache, updateValidationCache } from "./validation-cache.mjs";

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const uiRoot = join(repoRoot, "packages", "ui");
const srcRoot = join(uiRoot, "src");
pruneValidationCache();

function walk(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    return entry.isDirectory() ? walk(path) : [path];
  });
}

function parseArguments(argv) {
  const options = { cache: true, dryRun: false, explain: false, files: [] };
  for (let index = 0; index < argv.length; index += 1) {
    if (argv[index] === "--dry-run") options.dryRun = true;
    else if (argv[index] === "--no-cache") options.cache = false;
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
const tests = sourceFiles.filter((file) => /\.test\.tsx?$/.test(file));
const normalizedChanges = options.files.map((file) => file.replaceAll("\\", "/"));
const changedSources = new Set(normalizedChanges.filter((file) => file.startsWith("packages/ui/src/")).map((file) => resolve(repoRoot, file)));
const fileDigests = new Map();
function fileDigest(file) {
  if (!fileDigests.has(file)) {
    fileDigests.set(file, existsSync(file) ? createHash("sha256").update(readFileSync(file)).digest("hex") : "<missing>");
  }
  return fileDigests.get(file);
}
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
  if (/\.test\.tsx?$/.test(file)) {
    for (const match of source.matchAll(/["'`]([A-Za-z0-9_.-]+\.tsx?)["'`]/g)) {
      const dependency = resolve(srcRoot, match[1]);
      if (existsSync(dependency) && dependency !== file && !dependencies.includes(dependency)) dependencies.push(dependency);
    }
  }
  imports.set(file, dependencies);
}

let selected = [];
const selectionReasons = new Map();
const selectionInputs = new Map();
if (
  normalizedChanges.some((file) =>
    [
      "pnpm-lock.yaml",
      "pnpm-workspace.yaml",
      "packages/ui/package.json",
      "scripts/run-tsx-tests.mjs",
      "scripts/register-test-css.mjs",
      "scripts/test-changed-ui.mjs",
    ].includes(file),
  )
) {
  selected = tests;
  for (const test of tests) selectionReasons.set(test, "shared manifest or test runner changed");
} else {
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
      selectionInputs.set(test, new Set([changed]));
    }
  }
  const onlyTypeDependenciesChanged = changedSources.size > 0 && [...changedSources].every(
    (changed) => typeOnlyDependencies.has(changed) && !runtimeDependencies.has(changed),
  );
  if (changedSources.size > 0 && selected.length === 0 && !onlyTypeDependenciesChanged) {
    selected = tests;
    for (const test of tests) {
      selectionReasons.set(test, "unmapped UI source fallback");
      selectionInputs.set(test, new Set(changedSources));
    }
  }
}

function testInputHash(test) {
  const closure = new Set();
  function visit(file) {
    if (closure.has(file)) return;
    closure.add(file);
    for (const dependency of imports.get(file) ?? []) visit(dependency);
  }
  visit(test);
  const sharedInputs = [
    join(repoRoot, "package.json"),
    join(repoRoot, "pnpm-lock.yaml"),
    join(uiRoot, "package.json"),
    join(uiRoot, "tsconfig.json"),
    join(repoRoot, "scripts", "run-tsx-tests.mjs"),
    join(repoRoot, "scripts", "register-test-css.mjs"),
    join(repoRoot, "scripts", "test-changed-ui.mjs"),
    join(repoRoot, "scripts", "validation-cache.mjs"),
  ];
  const hash = createHash("sha256").update(`${process.version}\0${process.platform}\0${process.arch}\0${process.env.NODE_ENV ?? ""}\0${process.env.CI ?? ""}\0`);
  // Static import closure is sufficient for normal selection. Explicit extra
  // inputs cover colocated source-text tests and the conservative fallback.
  for (const file of [...closure, ...(selectionInputs.get(test) ?? []), ...sharedInputs].sort()) {
    hash.update(file);
    hash.update(fileDigest(file));
  }
  return hash.digest("hex");
}

const uiTestCache = readValidationCache("ui-test-cache");
const selectedHashes = new Map(selected.map((test) => [test, testInputHash(test)]));
if (options.cache) {
  const cached = selected.filter((test) => uiTestCache[relative(uiRoot, test).replaceAll("\\", "/")] === selectedHashes.get(test));
  selected = selected.filter((test) => !cached.includes(test));
  if (cached.length > 0) console.log(`[test:ui:changed] CACHED ${cached.length} selected test file(s).`);
}

if (selected.length === 0) {
  console.log("[test:ui:changed] No uncached UI unit tests are required.");
} else {
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
  } else {
    process.argv = [process.execPath, join(repoRoot, args[0]), ...args.slice(1)];
    await import(`./run-tsx-tests.mjs?selected=${Date.now()}`);
    updateValidationCache("ui-test-cache", (cache) => ({
      ...cache,
      ...Object.fromEntries(selected.map((test) => [relative(uiRoot, test).replaceAll("\\", "/"), selectedHashes.get(test)])),
    }));
  }
}
