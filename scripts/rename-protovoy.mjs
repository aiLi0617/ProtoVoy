/**
 * ProtoVoy bulk rename (text + file moves).
 * Dual-read compat for env/headers/storage/config is patched after this script.
 * Excludes: node_modules, target, dist, .git, .run, .pnpm-store, .target-review,
 * test-results, output, *.class, pnpm-lock (regenerated), plans under .cursor/plans
 */
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

const SKIP_DIR = new Set([
  "node_modules",
  "target",
  "dist",
  ".git",
  ".run",
  ".pnpm-store",
  ".target-review",
  "test-results",
  "output",
  "bin", // Java build output
  ".playwright-cli",
]);

const TEXT_EXT = new Set([
  ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".json", ".jsonc",
  ".rs", ".toml", ".md", ".mdx", ".css", ".scss", ".html", ".htm",
  ".yml", ".yaml", ".xml", ".properties", ".java", ".wit", ".svg",
  ".sh", ".ps1", ".bat", ".cmd", ".env", ".example", ".txt", ".gitignore",
  ".npmrc", ".editorconfig", ".lock",
]);

const TEXT_NAMES = new Set([
  "Dockerfile", "Makefile", "LICENSE", "NOTICE", "AGENTS.md", "SECURITY.md",
  "CODE_OF_CONDUCT.md", "CONTRIBUTING.md", "CHANGELOG.md", "README.md",
]);

/** Ordered replacements — longer / more specific first where needed. */
const REPLACEMENTS = [
  // npm scopes / packages
  ["@apivoy/", "@protovoy/"],
  // Java package
  ["dev.apivoy", "dev.protovoy"],
  // Bundle / identifiers
  ["com.apivoy.app", "com.protovoy.app"],
  ["com.apivoy.desktop", "com.protovoy.app"],
  // Env / Vite / window config
  ["VITE_APIVOY_", "VITE_PROTOVOY_"],
  ["__APIVOY_CONFIG__", "__PROTOVOY_CONFIG__"],
  ["APIVOY_", "PROTOVOY_"],
  // Headers (canonical casing variants)
  ["X-ApiVoy-", "X-ProtoVoy-"],
  ["x-apivoy-", "x-protovoy-"],
  // CSS vars & classes
  ["--apivoy-", "--protovoy-"],
  [".apivoy-", ".protovoy-"],
  ["apivoy-shell", "protovoy-shell"],
  // Monaco / MIME / DnD
  ["apivoy-dark", "protovoy-dark"],
  ["apivoy-light", "protovoy-light"],
  ["application/x-apivoy-item", "application/x-protovoy-item"],
  // Custom events & storage-ish prefixes (colon form)
  ["apivoy:", "protovoy:"],
  ["apivoy-agent-token", "protovoy:agent-token"],
  // WIT / plugin
  ["apivoy-plugin", "protovoy-plugin"],
  // Brand mark
  ["apivoy-mark", "protovoy-mark"],
  // Binary / service names
  ["apivoy-protocol-gateway", "protovoy-protocol-gateway"],
  ["apivoy-local-agent", "protovoy-local-agent"],
  ["apivoy-mock-server", "protovoy-mock-server"],
  ["apivoy-desktop", "protovoy-desktop"],
  ["apivoy_desktop_lib", "protovoy_desktop_lib"],
  ["apivoy-agent", "protovoy-agent"],
  ["apivoy-mock", "protovoy-mock"],
  ["apivoy-cli", "protovoy-cli"],
  ["apivoy-tools", "protovoy-tools"],
  ["apivoy-gateway", "protovoy-gateway"],
  ["apivoy-local.db", "protovoy-local.db"],
  ["apivoy-ticket.", "protovoy-ticket."],
  // Config dirs
  [".apivoy-gateway", ".protovoy-gateway"],
  [".apivoy", ".protovoy"],
  // Workspace / examples
  [".apivoy.json", ".protovoy.json"],
  // Component / providers
  ["ApiVoyProviders", "ProtoVoyProviders"],
  // Product / brand prose
  ["ApiVoy", "ProtoVoy"],
  ["APIVOY", "PROTOVOY"],
  // Remaining lowercase technical id (careful: after more specific patterns)
  ["apivoy", "protovoy"],
];

function shouldSkipDir(name) {
  return SKIP_DIR.has(name);
}

function isTextFile(filePath) {
  const base = path.basename(filePath);
  if (TEXT_NAMES.has(base)) return true;
  if (base.startsWith(".env")) return true;
  const ext = path.extname(base).toLowerCase();
  if (TEXT_EXT.has(ext)) return true;
  // extensionless scripts in scripts/
  if (!ext && filePath.includes(`${path.sep}scripts${path.sep}`)) return true;
  return false;
}

function walk(dir, out = []) {
  let entries;
  try {
    entries = fs.readdirSync(dir, { withFileTypes: true });
  } catch {
    return out;
  }
  for (const ent of entries) {
    if (ent.name === "." || ent.name === "..") continue;
    const full = path.join(dir, ent.name);
    if (ent.isDirectory()) {
      if (shouldSkipDir(ent.name)) continue;
      walk(full, out);
    } else if (ent.isFile()) {
      out.push(full);
    }
  }
  return out;
}

function transformContent(content, rel) {
  // Skip this script itself for the final apivoy→protovoy pass on its docs? Keep script readable.
  if (rel.replace(/\\/g, "/").endsWith("scripts/rename-protovoy.mjs")) {
    return { content, changed: false };
  }
  // Archive: still update for consistency (plan allows keep-or-update)
  let next = content;
  for (const [from, to] of REPLACEMENTS) {
    if (next.includes(from)) next = next.split(from).join(to);
  }
  return { content: next, changed: next !== content };
}

function renamePathIfNeeded(filePath) {
  const base = path.basename(filePath);
  const dir = path.dirname(filePath);
  let newBase = base;
  for (const [from, to] of [
    ["apivoy-mark.svg", "protovoy-mark.svg"],
    ["apivoy-plugin.wit", "protovoy-plugin.wit"],
    ["ApiVoyProviders.tsx", "ProtoVoyProviders.tsx"],
    [".apivoy.json", ".protovoy.json"],
  ]) {
    if (newBase.includes(from)) newBase = newBase.split(from).join(to);
  }
  if (newBase === base) return filePath;
  const dest = path.join(dir, newBase);
  if (fs.existsSync(dest)) {
    console.warn(`skip rename (exists): ${filePath} -> ${dest}`);
    return filePath;
  }
  fs.renameSync(filePath, dest);
  console.log(`renamed: ${path.relative(ROOT, filePath)} -> ${newBase}`);
  return dest;
}

function moveJavaPackage() {
  const srcRoot = path.join(ROOT, "services/collaboration-server/src");
  const moves = [];
  for (const tree of ["main/java", "test/java"]) {
    const from = path.join(srcRoot, tree, "dev/apivoy");
    const to = path.join(srcRoot, tree, "dev/protovoy");
    if (!fs.existsSync(from)) continue;
    fs.mkdirSync(path.dirname(to), { recursive: true });
    if (fs.existsSync(to)) {
      // merge by moving children
      for (const name of fs.readdirSync(from)) {
        const s = path.join(from, name);
        const d = path.join(to, name);
        if (fs.existsSync(d)) {
          console.warn(`java dest exists, skip: ${d}`);
        } else {
          fs.renameSync(s, d);
          moves.push(`${s} -> ${d}`);
        }
      }
      try { fs.rmdirSync(from); } catch { /* non-empty */ }
    } else {
      fs.renameSync(from, to);
      moves.push(`${from} -> ${to}`);
    }
    // clean empty apivoy parent
    const apivoyParent = path.join(srcRoot, tree, "dev/apivoy");
    try { fs.rmSync(apivoyParent, { recursive: true, force: true }); } catch { /* */ }
  }
  return moves;
}

function main() {
  console.log(`ProtoVoy rename in ${ROOT}`);
  const files = walk(ROOT);
  let changedFiles = 0;
  let renamed = 0;

  // First rename files so content edits hit new paths
  const afterRename = [];
  for (const f of files) {
    if (!isTextFile(f) && !f.endsWith(".svg") && !f.endsWith(".wit") && !f.endsWith(".json")) {
      // still consider rename for known patterns
    }
    const next = renamePathIfNeeded(f);
    if (next !== f) renamed++;
    afterRename.push(next);
  }

  for (const f of afterRename) {
    if (!isTextFile(f)) continue;
    const rel = path.relative(ROOT, f);
    let raw;
    try {
      raw = fs.readFileSync(f, "utf8");
    } catch {
      continue;
    }
    // Skip binary-looking
    if (raw.includes("\0")) continue;
    const { content, changed } = transformContent(raw, rel);
    if (changed) {
      fs.writeFileSync(f, content, "utf8");
      changedFiles++;
    }
  }

  const javaMoves = moveJavaPackage();
  console.log(`content files changed: ${changedFiles}`);
  console.log(`files renamed: ${renamed}`);
  console.log(`java moves: ${javaMoves.length}`);
  for (const m of javaMoves) console.log(`  ${m}`);
}

main();
