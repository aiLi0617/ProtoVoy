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
# Run only the browser specs mapped to changed UI features:
pnpm test:e2e:changed --base origin/main
```

The feature-to-spec mapping lives in `scripts/test-changed-e2e.mjs`. Add or
adjust a rule when introducing a new UI feature or browser spec. Shared and
unmapped UI files intentionally fall back to `e2e/web-smoke.spec.ts`. Logic-only
changes and ordinary direct e2e spec changes run desktop Chromium. CSS,
responsive shell changes, and specs listed in the selector's `mobileSpecs` set
also run the mobile project.

UI unit tests run in isolated processes with concurrency 4. Local runs stop
scheduling new tests after the first failure; CI collects all failures. The
underlying runner accepts `--fail-fast` and `--no-fail-fast` for an explicit
override.

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
