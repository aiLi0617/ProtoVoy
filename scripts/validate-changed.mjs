import { createHash } from "node:crypto";
import { existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { delimiter, dirname, join, relative, resolve, sep } from "node:path";
import { spawn, spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const cargoCommand = process.platform === "win32" ? "cargo.exe" : "cargo";
const snapshotPath = join(repoRoot, "node_modules", ".cache", "protovoy", "validate-changed.json");
const failurePath = join(repoRoot, "node_modules", ".cache", "protovoy", "validate-failures.json");

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
  const options = { base: undefined, dryRun: false, files: [], scopes: new Set(), sinceLastRun: false, staged: false };
  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index];
    if (argument === "--dry-run") options.dryRun = true;
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
  const tracked = base
    ? gitLines(["diff", "--name-only", "--diff-filter=ACDMRT", `${base}...HEAD`])
    : staged
      ? gitLines(["diff", "--cached", "--name-only", "--diff-filter=ACDMRT"])
      : gitLines(["diff", "--name-only", "--diff-filter=ACDMRT", "HEAD"]);
  const untracked = base || staged ? [] : gitLines(["ls-files", "--others", "--exclude-standard"]);
  return [...new Set([...tracked, ...untracked].map((file) => file.replaceAll("\\", "/")))];
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

function snapshotKey(scopes) {
  return [...scopes].sort().join(",");
}

function saveSnapshot(scopes, files) {
  const snapshots = readSnapshots();
  snapshots[snapshotKey(scopes)] = Object.fromEntries(files.map((file) => [file, fingerprint(file)]));
  mkdirSync(dirname(snapshotPath), { recursive: true });
  writeFileSync(snapshotPath, `${JSON.stringify(snapshots, null, 2)}\n`);
}

function readFailureStates() {
  if (!existsSync(failurePath)) return {};
  try {
    return JSON.parse(readFileSync(failurePath, "utf8"));
  } catch {
    return {};
  }
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
  const states = readFailureStates();
  states[key] = { pendingLabels: [...pendingLabels], files: Object.fromEntries(files.map((file) => [file, fingerprint(file)])) };
  mkdirSync(dirname(failurePath), { recursive: true });
  writeFileSync(failurePath, `${JSON.stringify(states, null, 2)}\n`);
}

function clearFailureState(key) {
  if (!existsSync(failurePath)) return;
  const states = readFailureStates();
  delete states[key];
  writeFileSync(failurePath, `${JSON.stringify(states, null, 2)}\n`);
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
    "scripts/run-tsx-test-shard.mjs",
    "scripts/test-changed-ui.mjs",
    "scripts/validate-changed.mjs",
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
  const manifests = [];
  for (const entry of readdirSync(join(repoRoot, "crates"), { withFileTypes: true })) {
    if (entry.isDirectory()) manifests.push(join(repoRoot, "crates", entry.name, "Cargo.toml"));
  }
  for (const entry of readdirSync(join(repoRoot, "apps"), { withFileTypes: true })) {
    if (!entry.isDirectory()) continue;
    for (const suffix of ["Cargo.toml", "src-tauri/Cargo.toml"]) {
      const manifest = join(repoRoot, "apps", entry.name, suffix);
      if (existsSync(manifest)) manifests.push(manifest);
    }
  }

  const packages = manifests.map((manifest) => {
    const content = readFileSync(manifest, "utf8");
    const packageStart = content.indexOf("[package]");
    const afterPackage = content.slice(packageStart + "[package]".length);
    const nextSection = afterPackage.search(/\n\[/);
    const packageSection = nextSection === -1 ? afterPackage : afterPackage.slice(0, nextSection);
    const name = packageSection.match(/^name\s*=\s*"([^"]+)"/m)?.[1];
    if (!name) throw new Error(`Unable to read package name from ${manifest}`);
    return {
      content,
      directory: relative(repoRoot, dirname(manifest)).split(sep).join("/"),
      name,
      dependencies: [],
    };
  });
  const names = new Set(packages.map((pkg) => pkg.name));
  for (const pkg of packages) {
    pkg.dependencies = [...names].filter(
      (name) =>
        name !== pkg.name &&
        new RegExp(`(^|\\n)${name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}\\s*=`, "m").test(pkg.content),
    );
    delete pkg.content;
  }
  return packages;
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
  };
}

async function runWorkspaceScripts(workspaces, scriptName, options, allowedLabels) {
  const tasks = workspaces
    .map((workspace) => workspaceScriptTask(workspace, scriptName))
    .filter((task) => task && (!allowedLabels || allowedLabels.has(task.label)));
  if (options.dryRun) {
    for (const task of tasks) {
      console.log(`[validate:changed] ${task.label}`);
      console.log(`[validate:changed] ${task.script}`);
    }
    return { completedLabels: [], failedLabels: [] };
  }
  let nextIndex = 0;
  let failed = false;
  const failFast = !process.env.CI;
  async function worker() {
    while (nextIndex < tasks.length && !(failFast && failed)) {
      const task = tasks[nextIndex++];
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
    }
  }
  await Promise.all(Array.from({ length: Math.min(4, tasks.length) }, () => worker()));
  return {
    completedLabels: tasks.filter((task) => task.resultStatus === 0).map((task) => task.label),
    failedLabels: tasks.filter((task) => task.resultStatus !== undefined && task.resultStatus !== 0).map((task) => task.label),
  };
}

function workspaceScriptLabels(workspaces, scriptName) {
  return workspaces.map((workspace) => workspaceScriptTask(workspace, scriptName)?.label).filter(Boolean);
}

function runSelectedUiTests(files, options) {
  const args = ["scripts/test-changed-ui.mjs", ...files.flatMap((file) => ["--file", file])];
  if (options.dryRun) {
    run(process.execPath, args, options);
    return true;
  }
  const result = spawnSync(process.execPath, args, { cwd: repoRoot, stdio: "inherit" });
  if (result.error) throw result.error;
  return result.status === 0;
}

async function main() {
  const options = parseArguments(process.argv.slice(2));
  const currentValidationKey = validationKey(options);
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
    const failureState = readFailureStates()[currentValidationKey];
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
      if (!options.dryRun) writeFailureState(currentValidationKey, files, pendingLabels);
      process.exit(1);
    }
    const tests = await runWorkspaceScripts(testedWorkspaces, "test", options, pendingLabels);
    tests.completedLabels.forEach((label) => pendingLabels.delete(label));
    if (tests.failedLabels.length > 0) {
      if (!options.dryRun) writeFailureState(currentValidationKey, files, pendingLabels);
      process.exit(1);
    }
    if (pendingLabels.has(uiLabel)) {
      if (!runSelectedUiTests(files, options)) {
        if (!options.dryRun) writeFailureState(currentValidationKey, files, pendingLabels);
        process.exit(1);
      }
      pendingLabels.delete(uiLabel);
    }
  }

  const rust = options.scopes.has("rust") ? rustPlan(files) : { full: false, direct: new Set(), affected: new Set() };
  if (rust.affected.size > 0) {
    if (rust.full) {
      run(cargoCommand, ["test", "--workspace", "--locked"], options);
      run(cargoCommand, ["clippy", "--workspace", "--all-targets", "--locked", "--", "-D", "warnings"], options);
    } else {
      const dependents = new Set([...rust.affected].filter((name) => !rust.direct.has(name)));
      run(cargoCommand, ["test", ...packages(rust.direct), "--locked"], options);
      if (dependents.size > 0) run(cargoCommand, ["check", ...packages(dependents), "--locked"], options);
      run(cargoCommand, ["clippy", ...packages(rust.direct), "--all-targets", "--locked", "--", "-D", "warnings"], options);
    }
  }

  const javaChanged = options.scopes.has("java") && files.some((file) => file.startsWith("services/collaboration-server/"));
  if (javaChanged) {
    const gradle = process.platform === "win32" ? "gradlew.bat" : "./gradlew";
    run(gradle, ["test", "--no-daemon"], {
      ...options,
      // The wrapper lives in the service directory.
      cwd: join(repoRoot, "services/collaboration-server"),
    });
  }

  if (js.affected.size === 0 && rust.affected.size === 0 && !javaChanged) {
    console.log("[validate:changed] No code validation is required for these files.");
  }
  if (!options.dryRun && tracksSnapshot) {
    saveSnapshot(options.scopes, dirtyFiles);
  }
  if (!options.dryRun) clearFailureState(currentValidationKey);
}

await main();
