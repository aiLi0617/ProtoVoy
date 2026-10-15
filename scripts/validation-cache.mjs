import { closeSync, existsSync, mkdirSync, openSync, readFileSync, readdirSync, renameSync, statSync, unlinkSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
export const validationCacheRoot = join(repoRoot, "node_modules", ".cache", "protovoy");
const schemaVersion = 2;

function cachePath(name) {
  return join(validationCacheRoot, `${name}.json`);
}

export function readValidationCache(name, fallback = {}) {
  const path = cachePath(name);
  try {
    const parsed = JSON.parse(readFileSync(path, "utf8"));
    return parsed?.schemaVersion === schemaVersion ? parsed.data : fallback;
  } catch {
    return fallback;
  }
}

export function writeValidationCache(name, data) {
  const path = cachePath(name);
  mkdirSync(dirname(path), { recursive: true });
  const lockPath = `${path}.lock`;
  let lock;
  for (let attempt = 0; attempt < 100; attempt += 1) {
    try {
      lock = openSync(lockPath, "wx");
      break;
    } catch (error) {
      if (error?.code !== "EEXIST") throw error;
      try {
        if (Date.now() - statSync(lockPath).mtimeMs > 30_000) unlinkSync(lockPath);
      } catch {
        // Another writer may have released the lock.
      }
      Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 10);
    }
  }
  if (lock === undefined) throw new Error(`Timed out acquiring validation cache lock: ${name}`);
  const temporaryPath = `${path}.${process.pid}.${Date.now()}.tmp`;
  try {
    writeFileSync(temporaryPath, `${JSON.stringify({ schemaVersion, data }, null, 2)}\n`);
    renameSync(temporaryPath, path);
  } finally {
    if (existsSync(temporaryPath)) unlinkSync(temporaryPath);
    closeSync(lock);
    if (existsSync(lockPath)) unlinkSync(lockPath);
  }
}

export function updateValidationCache(name, updater, fallback = {}) {
  const path = cachePath(name);
  mkdirSync(dirname(path), { recursive: true });
  const lockPath = `${path}.lock`;
  let lock;
  for (let attempt = 0; attempt < 100; attempt += 1) {
    try {
      lock = openSync(lockPath, "wx");
      break;
    } catch (error) {
      if (error?.code !== "EEXIST") throw error;
      try {
        if (Date.now() - statSync(lockPath).mtimeMs > 30_000) unlinkSync(lockPath);
      } catch {}
      Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 10);
    }
  }
  if (lock === undefined) throw new Error(`Timed out acquiring validation cache lock: ${name}`);
  const temporaryPath = `${path}.${process.pid}.${Date.now()}.tmp`;
  try {
    const updated = updater(readValidationCache(name, fallback));
    writeFileSync(temporaryPath, `${JSON.stringify({ schemaVersion, data: updated }, null, 2)}\n`);
    renameSync(temporaryPath, path);
    return updated;
  } finally {
    if (existsSync(temporaryPath)) unlinkSync(temporaryPath);
    closeSync(lock);
    if (existsSync(lockPath)) unlinkSync(lockPath);
  }
}

export function pruneValidationCache({ maxAgeDays = 30 } = {}) {
  const legacyPaths = [
    join(validationCacheRoot, "validate-failures.json"),
    join(validationCacheRoot, "e2e-server-5180.json"),
  ];
  for (const path of legacyPaths) {
    try {
      if (existsSync(path)) unlinkSync(path);
    } catch {
      // Cache cleanup must never block validation.
    }
  }
  try {
    for (const entry of readdirSync(validationCacheRoot)) {
      const path = join(validationCacheRoot, entry);
      if (/^e2e-server-\d+\.json$/.test(entry) || /^cache-self-test\.json$/.test(entry)) unlinkSync(path);
      else if (/\.(?:tmp|lock)$/.test(entry) && Date.now() - statSync(path).mtimeMs > 30_000) unlinkSync(path);
    }
  } catch {
    // The cache directory may not exist yet.
  }
  const cutoff = Date.now() - maxAgeDays * 24 * 60 * 60 * 1_000;
  try {
    for (const name of ["validate-changed", "test-changed-e2e", "validation-failures", "validate-task-cache", "ui-test-cache", "e2e-task-cache"]) {
      const path = cachePath(name);
      if (existsSync(path) && statSync(path).mtimeMs < cutoff) unlinkSync(path);
    }
  } catch {
    // Cache cleanup is best effort.
  }
}
