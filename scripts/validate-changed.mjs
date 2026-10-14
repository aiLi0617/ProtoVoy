import { existsSync, readFileSync, readdirSync } from "node:fs";
import { basename, dirname, join, relative, resolve, sep } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const invokedPnpmCli = process.env.npm_execpath && /pnpm/i.test(basename(process.env.npm_execpath))
  ? process.env.npm_execpath
  : undefined;
const pnpmCommand = invokedPnpmCli ? process.execPath : process.platform === "win32" ? "pnpm.cmd" : "pnpm";
const pnpmPrefix = invokedPnpmCli ? [invokedPnpmCli] : [];
const cargoCommand = process.platform === "win32" ? "cargo.exe" : "cargo";

function run(command, args, { capture = false, dryRun = false, cwd = repoRoot } = {}) {
  const display = [command, ...args].join(" ");
  if (dryRun) {
    console.log(`[validate:changed] ${display}`);
    return "";
  }

  console.log(`\n[validate:changed] ${display}`);
  const result = spawnSync(command, args, {
    cwd,
    encoding: "utf8",
    // Node 22 cannot execute Windows .cmd shims directly (EINVAL).
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
  const options = { base: undefined, dryRun: false, files: [], scopes: new Set() };
  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index];
    if (argument === "--dry-run") options.dryRun = true;
    else if (argument === "--base") options.base = argv[++index];
    else if (argument === "--file") options.files.push(argv[++index]);
    else if (["--js", "--rust", "--java"].includes(argument)) options.scopes.add(argument.slice(2));
    else throw new Error(`Unknown argument: ${argument}`);
  }
  if (options.base === undefined && argv.includes("--base")) {
    throw new Error("--base requires a git revision");
  }
  if (options.files.includes(undefined)) throw new Error("--file requires a repository-relative path");
  if (options.scopes.size === 0) options.scopes = new Set(["js", "rust", "java"]);
  return options;
}

function changedFiles(base) {
  const tracked = base
    ? gitLines(["diff", "--name-only", "--diff-filter=ACDMRT", `${base}...HEAD`])
    : gitLines(["diff", "--name-only", "--diff-filter=ACDMRT", "HEAD"]);
  const untracked = base ? [] : gitLines(["ls-files", "--others", "--exclude-standard"]);
  return [...new Set([...tracked, ...untracked].map((file) => file.replaceAll("\\", "/")))];
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

function jsPlan(files) {
  const workspaces = loadJsWorkspaces();
  const allNames = new Set(workspaces.map((workspace) => workspace.name));
  const globalInputs = new Set([
    "package.json",
    "pnpm-lock.yaml",
    "pnpm-workspace.yaml",
    ".npmrc",
    "scripts/run-tsx-tests.mjs",
    "scripts/test-changed-ui.mjs",
    "scripts/validate-changed.mjs",
  ]);
  if (files.some((file) => globalInputs.has(file))) {
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
  return { direct, affected: expandJsDependents(direct, workspaces) };
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
  if (rustInputs.length === 0) return { full: false, affected: new Set() };

  const workspaces = loadRustWorkspaces();
  if (
    rustInputs.some(
      (file) => file === "Cargo.toml" || file === "Cargo.lock" || file === "rust-toolchain.toml" || file.startsWith(".cargo/"),
    )
  ) {
    return { full: true, affected: new Set(workspaces.map((workspace) => workspace.name)) };
  }

  const affected = new Set(
    workspaces
      .filter((workspace) => rustInputs.some((file) => file.startsWith(`${workspace.directory}/`)))
      .map((workspace) => workspace.name),
  );
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
  return { full: false, affected };
}

function filters(names) {
  return [...names].sort().flatMap((name) => ["--filter", name]);
}

function packages(names) {
  return [...names].sort().flatMap((name) => ["--package", name]);
}

function main() {
  const options = parseArguments(process.argv.slice(2));
  const files = options.files.length > 0 ? options.files.map((file) => file.replaceAll("\\", "/")) : changedFiles(options.base);
  if (files.length === 0) {
    console.log("[validate:changed] No changed files.");
    return;
  }

  console.log(`[validate:changed] Inspecting ${files.length} changed file(s).`);
  const js = options.scopes.has("js") ? jsPlan(files) : { direct: new Set(), affected: new Set() };
  if (js.affected.size > 0) {
    run(pnpmCommand, [...pnpmPrefix, ...filters(js.affected), "-r", "typecheck"], options);
    const packageTests = new Set([...js.direct].filter((name) => name !== "@protovoy/ui"));
    if (packageTests.size > 0) run(pnpmCommand, [...pnpmPrefix, ...filters(packageTests), "-r", "test"], options);
    if (js.direct.has("@protovoy/ui")) {
      run(process.execPath, ["scripts/test-changed-ui.mjs", ...files.flatMap((file) => ["--file", file])], options);
    }
  }

  const rust = options.scopes.has("rust") ? rustPlan(files) : { full: false, affected: new Set() };
  if (rust.affected.size > 0) {
    const selection = rust.full ? ["--workspace"] : packages(rust.affected);
    run(cargoCommand, ["test", ...selection, "--locked"], options);
    run(cargoCommand, ["clippy", ...selection, "--all-targets", "--locked", "--", "-D", "warnings"], options);
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
}

main();
