import { createHash } from "node:crypto";
import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { delimiter, dirname, join, relative, resolve, sep } from "node:path";
import { homedir } from "node:os";
import { spawn, spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { pruneValidationCache, readValidationCache, updateValidationCache, writeValidationCache } from "./validation-cache.mjs";

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const cargoCommand = process.platform === "win32" && existsSync(join(homedir(), ".cargo", "bin", "cargo.exe"))
  ? join(homedir(), ".cargo", "bin", "cargo.exe")
  : process.platform === "win32" ? "cargo.exe" : "cargo";
const rustcCommand = process.platform === "win32" && existsSync(join(homedir(), ".cargo", "bin", "rustc.exe"))
  ? join(homedir(), ".cargo", "bin", "rustc.exe")
  : process.platform === "win32" ? "rustc.exe" : "rustc";

function clippyCommand() {
  if (process.platform !== "win32") return { argsPrefix: ["clippy"], command: cargoCommand };
  const sysroot = spawnSync(rustcCommand, ["--print", "sysroot"], { encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] }).stdout?.trim();
  const direct = sysroot ? join(sysroot, "bin", "cargo-clippy.exe") : undefined;
  return direct && existsSync(direct)
    ? { argsPrefix: ["clippy"], command: direct }
    : { argsPrefix: ["clippy"], command: cargoCommand };
}
pruneValidationCache();

function run(command, args, { capture = false, dryRun = false, cwd = repoRoot, env = process.env, shell } = {}) {
  const display = [command, ...args].join(" ");
  if (dryRun) {
    console.log(`[validate:changed] ${display}`);
    return "";
  }

  console.log(`\n[validate:changed] ${display}`);
  const result = spawnSync(command, args, {
    cwd,
    encoding: "utf8",
    env,
    // Node 22 cannot execute Windows .cmd/.bat shims directly (EINVAL).
    shell: shell ?? (process.platform === "win32" && /\.(?:cmd|bat)$/i.test(command)),
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
  const options = { base: undefined, cache: true, dryRun: false, files: [], plan: false, scopes: new Set(), sinceLastRun: false, staged: false };
  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index];
    if (argument === "--dry-run") options.dryRun = true;
    else if (argument === "--plan") {
      options.dryRun = true;
      options.plan = true;
    }
    else if (argument === "--no-cache") options.cache = false;
    else if (argument === "--base") options.base = argv[++index];
    else if (argument === "--file") options.files.push(argv[++index]);
    else if (argument === "--staged") options.staged = true;
    else if (argument === "--since-last-run") options.sinceLastRun = true;
    else if (["--js", "--rust", "--java"].includes(argument)) options.scopes.add(argument.slice(2));
    else throw new Error(`Unknown argument: ${argument}`);
  }
  if (options.base === undefined && argv.includes("--base")) {
    throw new Error("--base requires a git revision");
  }
  if (options.files.includes(undefined)) throw new Error("--file requires a repository-relative path");
  const selectors = [Boolean(options.base), options.files.length > 0, options.staged, options.sinceLastRun].filter(Boolean).length;
  if (selectors > 1) throw new Error("Use only one of --base, --file, --staged, or --since-last-run");
  if (options.scopes.size === 0) options.scopes = new Set(["js", "rust", "java"]);
  return options;
}

function changedFiles(base, staged = false) {
  if (base) return gitLines(["diff", "--name-only", "--diff-filter=ACDMRT", `${base}...HEAD`]);
  if (staged) return gitLines(["diff", "--cached", "--name-only", "--diff-filter=ACDMRT"]);
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
  return readValidationCache("validate-changed");
}

function snapshotKey(scopes) {
  return [...scopes].sort().join(",");
}

function saveSnapshot(scopes, files) {
  updateValidationCache("validate-changed", (snapshots) => ({
    ...snapshots,
    [snapshotKey(scopes)]: Object.fromEntries(files.map((file) => [file, fingerprint(file)])),
  }));
}

function readFailureStates() {
  return readValidationCache("validation-failures");
}

function validationKey(options) {
  const selector = options.base
    ? `base:${options.base}`
    : options.files.length > 0
      ? `files:${[...options.files].sort().join(",")}`
      : options.staged
        ? "staged"
        : options.sinceLastRun
          ? "since"
          : "dirty";
  return `${snapshotKey(options.scopes)}|${selector}`;
}

function writeFailureState(key, files, pendingLabels) {
  updateValidationCache("validation-failures", (states) => ({
    ...states,
    [key]: { pendingLabels: [...pendingLabels], files: Object.fromEntries(files.map((file) => [file, fingerprint(file)])) },
  }));
}

function clearFailureState(key) {
  updateValidationCache("validation-failures", (states) => {
    const updated = { ...states };
    delete updated[key];
    return updated;
  });
}

function runCommandStatus(command, args, { cwd = repoRoot, env = process.env, shell } = {}) {
  console.log(`\n[validate:changed] ${[command, ...args].join(" ")}`);
  const result = spawnSync(command, args, {
    cwd,
    env,
    shell: shell ?? (process.platform === "win32" && /\.(?:cmd|bat)$/i.test(command)),
    stdio: "inherit",
  });
  if (result.error) throw result.error;
  return result.status ?? 1;
}

function runRecoverableTasks(tasks, options, stateKey, files) {
  const plannedLabels = new Set(tasks.map((task) => task.label));
  const taskCache = readValidationCache("validate-task-cache");
  const failureState = readFailureStates()[stateKey];
  let pendingLabels = plannedLabels;
  if (failureState?.files && Array.isArray(failureState.pendingLabels)) {
    const filesUnchanged = files.every((file) => failureState.files[file] === fingerprint(file));
    if (filesUnchanged) {
      pendingLabels = new Set(failureState.pendingLabels.filter((label) => plannedLabels.has(label)));
      console.log(`[validate:changed] Retrying ${pendingLabels.size}/${plannedLabels.size} failed or not-yet-run task(s).`);
    }
  }
  for (const task of tasks) {
    if (!pendingLabels.has(task.label)) continue;
    if (task.inputHash && options.cache && taskCache[task.label] === task.inputHash) {
      console.log(`[validate:changed] CACHED ${task.label}`);
      pendingLabels.delete(task.label);
      continue;
    }
    if (options.dryRun) {
      console.log(`[validate:changed] RUN ${task.label}`);
      console.log(`[validate:changed] ${[task.command, ...task.args].join(" ")}`);
      continue;
    }
    if (runCommandStatus(task.command, task.args, task) !== 0) {
      writeFailureState(stateKey, files, pendingLabels);
      return false;
    }
    pendingLabels.delete(task.label);
    if (task.inputHash) {
      updateValidationCache("validate-task-cache", (current) => ({ ...current, [task.label]: task.inputHash }));
    }
    writeFailureState(stateKey, files, pendingLabels);
  }
  if (!options.dryRun) clearFailureState(stateKey);
  return true;
}

function loadJsWorkspaces() {
  const workspaces = [];
  for (const parent of ["apps", "packages"]) {
    for (const entry of readdirSync(join(repoRoot, parent), { withFileTypes: true })) {
      if (!entry.isDirectory()) continue;
      const directory = `${parent}/${entry.name}`;
      const manifest = join(repoRoot, directory, "package.json");
      if (!existsSync(manifest)) continue;
      const packageJson = JSON.parse(readFileSync(manifest, "utf8"));
      workspaces.push({ directory, name: packageJson.name, packageJson });
    }
  }
  return workspaces;
}

const directoryFiles = new Map();
const fileDigests = new Map();

function walkFiles(directory) {
  if (directoryFiles.has(directory)) return directoryFiles.get(directory);
  const result = [];
  if (!existsSync(directory)) {
    directoryFiles.set(directory, result);
    return result;
  }
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    if (["node_modules", "dist", "target", "coverage", ".turbo"].includes(entry.name)) continue;
    const path = join(directory, entry.name);
    if (entry.isDirectory()) result.push(...walkFiles(path));
    else if (entry.isFile() && !/\.(?:tsbuildinfo|log)$/.test(entry.name)) result.push(path);
  }
  directoryFiles.set(directory, result);
  return result;
}

function fileDigest(file) {
  if (fileDigests.has(file)) return fileDigests.get(file);
  const digest = existsSync(file) && statSync(file).isFile()
    ? createHash("sha256").update(readFileSync(file)).digest("hex")
    : "<missing>";
  fileDigests.set(file, digest);
  return digest;
}

function hashFiles(files, seed = "") {
  const hash = createHash("sha256").update(seed);
  for (const file of [...new Set(files)].sort()) {
    hash.update(relative(repoRoot, file).split(sep).join("/"));
    hash.update(fileDigest(file));
  }
  return hash.digest("hex");
}

function workspaceDependencies(workspace, workspaces) {
  const names = new Set(workspaces.map((candidate) => candidate.name));
  const dependencies = {
    ...workspace.packageJson.dependencies,
    ...workspace.packageJson.devDependencies,
    ...workspace.packageJson.peerDependencies,
    ...workspace.packageJson.optionalDependencies,
  };
  return Object.keys(dependencies).filter((name) => names.has(name));
}

function workspaceTaskHash(task, workspaces) {
  const byName = new Map(workspaces.map((workspace) => [workspace.name, workspace]));
  const included = new Set();
  function include(name) {
    if (included.has(name)) return;
    included.add(name);
    const workspace = byName.get(name);
    if (!workspace) return;
    workspaceDependencies(workspace, workspaces).forEach(include);
  }
  include(task.workspace.name);
  const files = [...included].flatMap((name) => walkFiles(join(repoRoot, byName.get(name).directory)));
  for (const rootInput of ["package.json", "pnpm-lock.yaml", "pnpm-workspace.yaml", ".npmrc", "scripts/run-tsx-tests.mjs", "scripts/register-test-css.mjs", "scripts/test-changed-ui.mjs", "scripts/validate-changed.mjs", "scripts/validation-cache.mjs"]) {
    files.push(join(repoRoot, rootInput));
  }
  return hashFiles(files, `${process.version}\0${process.platform}\0${process.arch}\0${process.env.NODE_ENV ?? ""}\0${process.env.CI ?? ""}\0${process.env.NODE_OPTIONS ?? ""}\0${task.script}\0`);
}

function gitFile(ref, file) {
  const result = spawnSync("git", ["show", `${ref}:${file}`], {
    cwd: repoRoot,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "ignore"],
  });
  return result.status === 0 ? result.stdout : undefined;
}

function rootManifestAffectsWorkspaces(options) {
  const previousText = gitFile(options.base ?? "HEAD", "package.json");
  if (!previousText) return true;
  try {
    const previous = JSON.parse(previousText);
    const current = JSON.parse(readFileSync(join(repoRoot, "package.json"), "utf8"));
    const workspaceFields = [
      "packageManager",
      "engines",
      "dependencies",
      "devDependencies",
      "peerDependencies",
      "optionalDependencies",
      "pnpm",
    ];
    return workspaceFields.some((field) => JSON.stringify(previous[field]) !== JSON.stringify(current[field]));
  } catch {
    return true;
  }
}

function expandJsDependents(selected, workspaces) {
  const affected = new Set(selected);
  let changed = true;
  while (changed) {
    changed = false;
    for (const workspace of workspaces) {
      if (affected.has(workspace.name)) continue;
      const dependencies = {
        ...workspace.packageJson.dependencies,
        ...workspace.packageJson.devDependencies,
        ...workspace.packageJson.peerDependencies,
        ...workspace.packageJson.optionalDependencies,
      };
      if (Object.keys(dependencies).some((dependency) => affected.has(dependency))) {
        affected.add(workspace.name);
        changed = true;
      }
    }
  }
  return affected;
}

function exportedWorkspaceFiles(workspace) {
  const entries = new Set();
  function add(value) {
    if (typeof value === "string" && value.startsWith("./")) entries.add(resolve(repoRoot, workspace.directory, value));
    else if (value && typeof value === "object") Object.values(value).forEach(add);
  }
  add(workspace.packageJson.main);
  add(workspace.packageJson.types);
  add(workspace.packageJson.exports);
  const reachable = new Set();
  function visit(file) {
    const candidates = [file, `${file}.ts`, `${file}.tsx`, join(file, "index.ts"), join(file, "index.tsx")];
    const resolvedFile = candidates.find((candidate) => existsSync(candidate));
    if (!resolvedFile || reachable.has(resolvedFile)) return;
    reachable.add(resolvedFile);
    const source = readFileSync(resolvedFile, "utf8");
    for (const match of source.matchAll(/(?:from\s*|import\s*)["']([^"']+)["']/g)) {
      if (match[1].startsWith(".")) visit(resolve(dirname(resolvedFile), match[1]));
    }
  }
  entries.forEach(visit);
  return reachable;
}

function workspacePublicSurfaceChanged(workspace, files) {
  if (files.some((file) => file === `${workspace.directory}/package.json` || file === `${workspace.directory}/tsconfig.json`)) return true;
  const exportedFiles = exportedWorkspaceFiles(workspace);
  return files.some((file) => exportedFiles.has(resolve(repoRoot, file)));
}

function jsPlan(files, options) {
  const workspaces = loadJsWorkspaces();
  const allNames = new Set(workspaces.map((workspace) => workspace.name));
  const globalInputs = new Set([
    "pnpm-lock.yaml",
    "pnpm-workspace.yaml",
    ".npmrc",
    "scripts/run-tsx-tests.mjs",
    "scripts/register-test-css.mjs",
    "scripts/test-changed-ui.mjs",
    "scripts/validate-changed.mjs",
    "scripts/validation-cache.mjs",
  ]);
  if (files.some((file) => globalInputs.has(file)) || (files.includes("package.json") && rootManifestAffectsWorkspaces(options))) {
    return { direct: allNames, affected: allNames };
  }

  const direct = new Set();
  for (const workspace of workspaces) {
    if (
      files.some(
        (file) =>
          file.startsWith(`${workspace.directory}/`) &&
          !(workspace.directory === "apps/desktop" && file.startsWith("apps/desktop/src-tauri/")),
      )
    ) {
      direct.add(workspace.name);
    }
  }
  const publicChanges = new Set(
    workspaces
      .filter((workspace) => direct.has(workspace.name) && workspacePublicSurfaceChanged(workspace, files))
      .map((workspace) => workspace.name),
  );
  const affected = expandJsDependents(publicChanges, workspaces);
  direct.forEach((name) => affected.add(name));
  return { direct, affected };
}

function loadRustWorkspaces() {
  const metadataInputs = [
    join(repoRoot, "Cargo.toml"),
    join(repoRoot, "Cargo.lock"),
    join(repoRoot, "rust-toolchain.toml"),
    ...walkFiles(join(repoRoot, ".cargo")),
    ...walkFiles(join(repoRoot, "crates")).filter((file) => file.endsWith("Cargo.toml")),
    ...walkFiles(join(repoRoot, "apps")).filter((file) => file.endsWith("Cargo.toml")),
  ];
  const inputHash = hashFiles(metadataInputs, `${process.version}\0cargo-metadata-v2\0`);
  const cached = readValidationCache("cargo-workspace-metadata");
  if (cached.inputHash === inputHash && Array.isArray(cached.workspaces)) return cached.workspaces;

  const result = spawnSync(cargoCommand, ["metadata", "--format-version", "1", "--no-deps", "--offline"], {
    cwd: repoRoot,
    encoding: "utf8",
    maxBuffer: 64 * 1024 * 1024,
    stdio: ["ignore", "pipe", "inherit"],
  });
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error("cargo metadata failed; dependency-aware Rust validation could not be planned.");
  const metadata = JSON.parse(result.stdout);
  const workspaceIds = new Set(metadata.workspace_members);
  const workspacePackages = metadata.packages.filter((pkg) => workspaceIds.has(pkg.id));
  const workspaceNames = new Set(workspacePackages.map((pkg) => pkg.name));
  const workspaces = workspacePackages.map((pkg) => ({
    dependencies: (pkg.dependencies ?? [])
      .map((dependency) => dependency.name)
      .filter((name) => workspaceNames.has(name)),
    directory: relative(repoRoot, dirname(pkg.manifest_path)).split(sep).join("/"),
    name: pkg.name,
  }));
  writeValidationCache("cargo-workspace-metadata", { inputHash, workspaces });
  return workspaces;
}

function rustPlan(files) {
  const rustInputs = files.filter(
    (file) =>
      file.endsWith(".rs") ||
      file.endsWith("Cargo.toml") ||
      file === "Cargo.lock" ||
      file === "rust-toolchain.toml" ||
      file.startsWith(".cargo/"),
  );
  if (rustInputs.length === 0) return { full: false, direct: new Set(), affected: new Set() };

  const workspaces = loadRustWorkspaces();
  if (
    rustInputs.some(
      (file) => file === "Cargo.toml" || file === "Cargo.lock" || file === "rust-toolchain.toml" || file.startsWith(".cargo/"),
    )
  ) {
    const all = new Set(workspaces.map((workspace) => workspace.name));
    return { full: true, direct: all, affected: all };
  }

  const direct = new Set(
    workspaces
      .filter((workspace) => rustInputs.some((file) => file.startsWith(`${workspace.directory}/`)))
      .map((workspace) => workspace.name),
  );
  const affected = new Set(direct);
  let changed = true;
  while (changed) {
    changed = false;
    for (const workspace of workspaces) {
      if (affected.has(workspace.name)) continue;
      if (workspace.dependencies.some((dependency) => affected.has(dependency))) {
        affected.add(workspace.name);
        changed = true;
      }
    }
  }
  return { full: false, direct, affected };
}

let cargoVersion;
function rustTaskHash(names, args) {
  const workspaces = loadRustWorkspaces();
  const byName = new Map(workspaces.map((workspace) => [workspace.name, workspace]));
  const included = new Set();
  function include(name) {
    if (included.has(name)) return;
    included.add(name);
    byName.get(name)?.dependencies.forEach(include);
  }
  names.forEach(include);
  const inputs = [
    join(repoRoot, "Cargo.toml"),
    join(repoRoot, "Cargo.lock"),
    join(repoRoot, "rust-toolchain.toml"),
    ...walkFiles(join(repoRoot, ".cargo")),
    join(repoRoot, "scripts", "validate-changed.mjs"),
    join(repoRoot, "scripts", "validation-cache.mjs"),
    ...[...included].flatMap((name) => walkFiles(join(repoRoot, byName.get(name).directory))),
  ];
  cargoVersion ??= spawnSync(cargoCommand, ["--version"], { cwd: repoRoot, encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] }).stdout?.trim() ?? "cargo";
  return hashFiles(inputs, `${process.platform}\0${process.arch}\0${cargoVersion}\0${process.env.RUSTFLAGS ?? ""}\0${process.env.RUSTDOCFLAGS ?? ""}\0${process.env.CARGO_BUILD_TARGET ?? ""}\0${args.join("\0")}\0`);
}

function packages(names) {
  return [...names].sort().flatMap((name) => ["--package", name]);
}

function workspaceScriptTask(workspace, scriptName) {
  const script = workspace.packageJson.scripts?.[scriptName];
  if (!script) return undefined;
  const binPaths = [
    join(repoRoot, workspace.directory, "node_modules", ".bin"),
    join(repoRoot, "node_modules", ".bin"),
  ];
  return {
    cwd: join(repoRoot, workspace.directory),
    env: { ...process.env, PATH: `${binPaths.join(delimiter)}${delimiter}${process.env.PATH ?? ""}` },
    label: `${workspace.name} -> ${scriptName}`,
    script,
    workspace,
  };
}

async function runWorkspaceScripts(workspaces, scriptName, options, allowedLabels) {
  const taskCache = readValidationCache("validate-task-cache");
  const tasks = workspaces
    .map((workspace) => workspaceScriptTask(workspace, scriptName))
    .filter((task) => task && (!allowedLabels || allowedLabels.has(task.label)));
  for (const task of tasks) {
    task.inputHash = workspaceTaskHash(task, loadJsWorkspaces());
    task.cached = options.cache && taskCache[task.label] === task.inputHash;
  }
  if (options.dryRun) {
    for (const task of tasks) {
      console.log(`[validate:changed] ${task.cached ? "CACHED" : "RUN"} ${task.label}`);
      if (!task.cached) console.log(`[validate:changed] ${task.script}`);
    }
    return { completedLabels: [], failedLabels: [] };
  }
  for (const task of tasks.filter((candidate) => candidate.cached)) {
    task.resultStatus = 0;
    console.log(`[validate:changed] CACHED ${task.label}`);
  }
  const runnableTasks = tasks.filter((task) => !task.cached);
  let nextIndex = 0;
  let failed = false;
  const failFast = !process.env.CI;
  async function worker() {
    while (nextIndex < runnableTasks.length && !(failFast && failed)) {
      const task = runnableTasks[nextIndex++];
      const result = await new Promise((resolveTask) => {
        const child = spawn(task.script, [], { cwd: task.cwd, env: task.env, shell: true });
        let stdout = "";
        let stderr = "";
        child.stdout.on("data", (chunk) => (stdout += chunk));
        child.stderr.on("data", (chunk) => (stderr += chunk));
        child.on("error", (error) => resolveTask({ error, status: 1, stderr, stdout }));
        child.on("close", (status) => resolveTask({ status: status ?? 1, stderr, stdout }));
      });
      console.log(`\n[validate:changed] ${result.status === 0 ? "PASS" : "FAIL"} ${task.label}`);
      task.resultStatus = result.status;
      if (result.stdout) process.stdout.write(result.stdout);
      if (result.stderr) process.stderr.write(result.stderr);
      if (result.error) console.error(result.error);
      if (result.status !== 0) failed = true;
      else taskCache[task.label] = task.inputHash;
    }
  }
  await Promise.all(Array.from({ length: Math.min(4, runnableTasks.length) }, () => worker()));
  if (runnableTasks.some((task) => task.resultStatus === 0)) {
    updateValidationCache("validate-task-cache", (current) => ({ ...current, ...taskCache }));
  }
  return {
    completedLabels: tasks.filter((task) => task.resultStatus === 0).map((task) => task.label),
    failedLabels: tasks.filter((task) => task.resultStatus !== undefined && task.resultStatus !== 0).map((task) => task.label),
  };
}

function workspaceScriptLabels(workspaces, scriptName) {
  return workspaces.map((workspace) => workspaceScriptTask(workspace, scriptName)?.label).filter(Boolean);
}

async function runSelectedUiTests(files, options) {
  const args = ["scripts/test-changed-ui.mjs", ...(!options.cache ? ["--no-cache"] : []), ...files.flatMap((file) => ["--file", file])];
  const uiWorkspace = loadJsWorkspaces().find((workspace) => workspace.name === "@protovoy/ui");
  const task = { label: "@protovoy/ui -> selected-tests", script: args.join(" "), workspace: uiWorkspace };
  const cacheKey = `${task.label}|${createHash("sha256").update(task.script).digest("hex")}`;
  const inputHash = workspaceTaskHash(task, loadJsWorkspaces());
  const taskCache = readValidationCache("validate-task-cache");
  if (options.cache && taskCache[cacheKey] === inputHash) {
    console.log(`[validate:changed] CACHED ${task.label}`);
    return true;
  }
  if (options.dryRun) {
    console.log(`[validate:changed] RUN ${task.label}`);
    run(process.execPath, args, options);
    return true;
  }
  process.argv = [process.execPath, join(repoRoot, args[0]), ...args.slice(1)];
  await import(`./test-changed-ui.mjs?validation=${Date.now()}`);
  taskCache[task.label] = inputHash;
  updateValidationCache("validate-task-cache", (current) => ({ ...current, [cacheKey]: inputHash }));
  return true;
}

async function main() {
  const options = parseArguments(process.argv.slice(2));
  const currentValidationKey = validationKey(options);
  const jsFailureKey = `validate:${currentValidationKey}:js`;
  const rustFailureKey = `validate:${currentValidationKey}:rust`;
  const javaFailureKey = `validate:${currentValidationKey}:java`;
  const tracksSnapshot = !options.base && options.files.length === 0 && !options.staged;
  const dirtyFiles = tracksSnapshot ? changedFiles(undefined) : [];
  let files = options.files.length > 0
    ? options.files.map((file) => file.replaceAll("\\", "/"))
    : tracksSnapshot
      ? dirtyFiles
      : changedFiles(options.base, options.staged);
  if (options.sinceLastRun) {
    const previous = readSnapshots()[snapshotKey(options.scopes)] ?? {};
    files = dirtyFiles.filter((file) => previous[file] !== fingerprint(file));
    console.log(`[validate:changed] ${files.length}/${dirtyFiles.length} dirty file(s) changed since the last successful matching run.`);
  }
  if (files.length === 0) {
    console.log("[validate:changed] No changed files.");
    if (tracksSnapshot && !options.dryRun) saveSnapshot(options.scopes, dirtyFiles);
    return;
  }

  console.log(`[validate:changed] Inspecting ${files.length} changed file(s).`);
  if (options.plan) files.forEach((file) => console.log(`[validate:plan] changed: ${file}`));
  const js = options.scopes.has("js") ? jsPlan(files, options) : { direct: new Set(), affected: new Set() };
  if (js.affected.size > 0) {
    const workspaces = loadJsWorkspaces();
    const affectedWorkspaces = workspaces.filter((candidate) => js.affected.has(candidate.name)).sort((left, right) => left.name.localeCompare(right.name));
    const packageTests = new Set([...js.direct].filter((name) => name !== "@protovoy/ui"));
    const testedWorkspaces = workspaces.filter((candidate) => packageTests.has(candidate.name)).sort((left, right) => left.name.localeCompare(right.name));
    const uiLabel = "@protovoy/ui -> selected-tests";
    const plannedLabels = new Set([
      ...workspaceScriptLabels(affectedWorkspaces, "typecheck"),
      ...workspaceScriptLabels(testedWorkspaces, "test"),
      ...(js.direct.has("@protovoy/ui") ? [uiLabel] : []),
    ]);
    const failureState = readFailureStates()[jsFailureKey];
    let pendingLabels = new Set(plannedLabels);
    if (options.scopes.has("js") && failureState?.files && Array.isArray(failureState.pendingLabels)) {
      pendingLabels = new Set(failureState.pendingLabels.filter((label) => plannedLabels.has(label)));
      const changedSinceFailure = files.filter((file) => failureState.files[file] !== fingerprint(file));
      const retryPlan = jsPlan(changedSinceFailure, options);
      retryPlan.affected.forEach((name) => pendingLabels.add(`${name} -> typecheck`));
      retryPlan.direct.forEach((name) => pendingLabels.add(name === "@protovoy/ui" ? uiLabel : `${name} -> test`));
      console.log(`[validate:changed] Retrying ${pendingLabels.size}/${plannedLabels.size} pending or newly affected JavaScript task(s).`);
    }
    const typechecks = await runWorkspaceScripts(affectedWorkspaces, "typecheck", options, pendingLabels);
    typechecks.completedLabels.forEach((label) => pendingLabels.delete(label));
    if (typechecks.failedLabels.length > 0) {
      if (!options.dryRun) writeFailureState(jsFailureKey, files, pendingLabels);
      process.exit(1);
    }
    const tests = await runWorkspaceScripts(testedWorkspaces, "test", options, pendingLabels);
    tests.completedLabels.forEach((label) => pendingLabels.delete(label));
    if (tests.failedLabels.length > 0) {
      if (!options.dryRun) writeFailureState(jsFailureKey, files, pendingLabels);
      process.exit(1);
    }
    if (pendingLabels.has(uiLabel)) {
      if (!await runSelectedUiTests(files, options)) {
        if (!options.dryRun) writeFailureState(jsFailureKey, files, pendingLabels);
        process.exit(1);
      }
      pendingLabels.delete(uiLabel);
    }
    if (!options.dryRun) clearFailureState(jsFailureKey);
  }

  const rust = options.scopes.has("rust") ? rustPlan(files) : { full: false, direct: new Set(), affected: new Set() };
  if (rust.affected.size > 0) {
    const rustTasks = [];
    const addRustTask = (label, args, names, command = cargoCommand) => rustTasks.push({
      label,
      command,
      args,
      inputHash: rustTaskHash(names, [command, ...args]),
    });
    const clippy = clippyCommand();
    if (rust.full) {
      addRustTask("rust -> workspace test", ["test", "--workspace", "--locked"], rust.affected);
      addRustTask("rust -> workspace clippy", [...clippy.argsPrefix, "--workspace", "--all-targets", "--locked", "--", "-D", "warnings"], rust.affected, clippy.command);
    } else {
      const dependents = new Set([...rust.affected].filter((name) => !rust.direct.has(name)));
      addRustTask(`rust -> test ${[...rust.direct].sort().join(",")}`, ["test", ...packages(rust.direct), "--locked"], rust.direct);
      if (dependents.size > 0) addRustTask(`rust -> check dependents of ${[...rust.direct].sort().join(",")}`, ["check", ...packages(dependents), "--locked"], rust.affected);
      addRustTask(`rust -> clippy ${[...rust.direct].sort().join(",")}`, [...clippy.argsPrefix, ...packages(rust.direct), "--all-targets", "--locked", "--", "-D", "warnings"], rust.direct, clippy.command);
    }
    if (!runRecoverableTasks(rustTasks, options, rustFailureKey, files)) process.exit(1);
  }

  const javaChanged = options.scopes.has("java") && files.some((file) => file.startsWith("services/collaboration-server/"));
  if (javaChanged) {
    const gradle = process.platform === "win32" ? "gradlew.bat" : "./gradlew";
    const javaTask = {
      label: "collaboration-server -> test",
      command: gradle,
      args: ["test", "--no-daemon"],
      cwd: join(repoRoot, "services/collaboration-server"),
    };
    if (!runRecoverableTasks([javaTask], options, javaFailureKey, files)) process.exit(1);
  }

  if (js.affected.size === 0 && rust.affected.size === 0 && !javaChanged) {
    console.log("[validate:changed] No code validation is required for these files.");
  }
  if (!options.dryRun && tracksSnapshot) {
    saveSnapshot(options.scopes, dirtyFiles);
  }
}

await main();
