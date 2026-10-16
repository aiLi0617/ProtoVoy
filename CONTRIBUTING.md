# Contributing to ProtoVoy

Thank you for your interest in contributing to ProtoVoy.

## Before contributing

- Search existing [issues](https://github.com/aiLi0617/ProtoVoy/issues) and pull requests.
- For large changes, open a proposal issue first.
- Keep changes focused on one responsibility.
- Do not include credentials, private endpoints, or proprietary code.

## Development setup

### Requirements

- **Node.js** 22+
- **pnpm** 10.27+
- **Rust** stable (with platform-specific [Tauri dependencies](https://v2.tauri.app/start/prerequisites/))
- **Java 21** — only when working on `services/collaboration-server/`

### Clone and run

```bash
git clone https://github.com/aiLi0617/ProtoVoy.git
cd ProtoVoy
pnpm install
cargo check --workspace

# Desktop
pnpm dev:desktop

# Web (requires Local Agent in another terminal)
cargo run -p protovoy-local-agent
pnpm dev:web

# CLI smoke test
cargo run -p protovoy-cli -- http-get https://httpbin.org/get
```

### Collaboration server (optional)

```bash
cd services/collaboration-server
./gradlew.bat test   # Windows
./gradlew test       # macOS / Linux
```

## Validation

For the normal edit/test loop, validate only changed workspaces and their
in-repository dependents:

```bash
pnpm validate:changed
# Compare a committed branch with its base:
pnpm validate:changed --base origin/main
# Limit a CI job or local check to one toolchain:
pnpm validate:changed --js --base origin/main
# Validate only paths currently staged in Git:
pnpm validate:staged
# During a tight edit loop, validate only dirty contents changed since success:
pnpm validate:since
# Run only the browser specs mapped to changed UI features:
pnpm test:e2e:changed --base origin/main
# Restrict browser selection to staged paths or changes since the last success:
pnpm test:e2e:staged
pnpm test:e2e:since
```

If Corepack is unavailable or offline, the validation script no longer needs
pnpm internally and can be started with `npm run validate:changed`,
`npm run validate:staged`, or `npm run validate:since`. Workspace scripts are
run directly with up to four concurrent isolated processes.
The changed-E2E selector invokes the installed Playwright CLI directly and
`playwright.runtime.config.ts` starts the installed Vite CLI through Node, so local browser tests
do not need Corepack after dependencies are installed. The E2E runners also bypass
HTTP proxies for `localhost` and `127.0.0.1`, while preserving existing proxy bypasses.
`test:e2e:since` records its snapshot only after a successful matching run, so a
failed browser test remains selected after the implementation is fixed.
Root `package.json` script-only edits do not expand UI validation to every
workspace; dependency, package-manager, engine, override, and lockfile changes
remain full-workspace inputs.

The feature-to-spec mapping lives in `scripts/test-changed-e2e.mjs`. Add or
adjust a rule when introducing a new UI feature or browser spec. Shared and
unmapped UI files intentionally fall back to `e2e/web-smoke.spec.ts`. Logic-only
changes and ordinary direct e2e spec changes run desktop Chromium. CSS,
responsive shell changes, and specs listed in the selector's `mobileSpecs` set
also run the mobile project.

UI unit tests run in isolated processes with concurrency 4. Local runs stop
scheduling new tests after the first failure; CI collects all failures. The
underlying runner accepts `--fail-fast` and `--no-fail-fast` for an explicit
override. Local duration history is used to balance slow tests across shards,
reducing the parallel suite's long tail.
The runner packs selected files into four duration-balanced shard processes,
avoiding one TSX/Node startup per file while keeping cross-shard isolation.
Use `node scripts/test-changed-ui.mjs --dry-run --explain --file <path>` to see
the runtime import path that selected each UI test. Pure `import type` edges are
covered by typecheck and no longer expand the runtime unit-test set.

TypeScript workspaces keep incremental build metadata under
`node_modules/.cache/protovoy/tsbuildinfo`. `validate:since` stores content
fingerprints in the same ignored cache tree; deleting the cache safely causes
the next run to validate all currently dirty files.
If workspace validation fails, the next run prioritizes the failed tasks and
tasks newly affected by the fix, instead of repeating unrelated successful
workspace checks.

Downstream JavaScript workspace typechecks expand only when a changed file is
on a package's exported entry graph. Tests and private implementation files stay
within their owning workspace; manifests and shared toolchain inputs remain
conservative full-workspace triggers.

For repeated browser-test development, `pnpm test:e2e:ui` keeps Playwright and
the Vite server alive while tests are rerun. Alternatively keep
`pnpm test:e2e:server` running in one terminal; normal E2E commands detect and
reuse that server instead of starting another one.

Routine Rust validation runs tests and Clippy only for directly changed crates,
using `cargo check` for their dependents. Shared Cargo manifests, the lockfile,
and toolchain configuration still trigger full-workspace validation.

CI uses the same affected-workspace check for pull requests. Before merging to
`main` or cutting a release, run the complete validation once:

```bash
pnpm typecheck
pnpm test
pnpm test:e2e
cargo test --workspace --locked
cargo clippy --workspace --all-targets --locked -- -D warnings
```

## Pull requests

A pull request should include:

- The problem being solved
- The implementation approach
- Tests (or explain why not applicable)
- Documentation updates for behavior changes
- Screenshots for UI changes
- Security implications when touching secrets, authentication, TLS, scripts, plugins, proxying, or protocol parsing

Before requesting review, self-check any touched areas against
[docs/maintainers/PR_REVIEW_CHECKLIST.md](docs/maintainers/PR_REVIEW_CHECKLIST.md)
(dependency audit, CodeQL-prone UI/import patterns, Agent health contract, workflow assertions).

See [AGENTS.md](AGENTS.md) for maintainer automation rules and [SECURITY.md](SECURITY.md) for vulnerability reporting.
