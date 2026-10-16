# AGENTS.md

## Repository goals

ProtoVoy is a local-first multi-protocol API debugging client.

## Validation strategy

During implementation:

- Run the smallest relevant test or package-level typecheck after changes.
- Do not rerun unchanged full-workspace suites after every edit.
- Group related edits before validation.
- `pnpm validate:changed` checks changed JavaScript/TypeScript and Rust workspaces
  plus their in-repository dependents. Use `--base <git-ref>` to validate a branch.
  JavaScript workspace scripts run directly with concurrency four, so
  `npm run validate:changed` remains available when Corepack is offline.
- `pnpm validate:staged` limits validation to staged paths. `pnpm validate:since`
  validates dirty files whose contents changed since the last successful matching run.
- `pnpm test:e2e:changed` runs browser specs mapped to changed UI features. It
  falls back to the browser smoke suite for shared or unmapped UI source files.
  The selector uses `playwright.managed.config.ts`; `scripts/run-e2e.mjs` hosts
  Vite in-process so browser tests remain runnable without Corepack and do not
  leave shell-launched services behind on Windows.
  Use `pnpm test:e2e:staged` for staged paths or `pnpm test:e2e:since` to rerun
  only dirty contents changed since the last successful matching browser run.
  Successful matching browser runs are content-cached locally; pass `--no-cache`
  when an intentional rerun is required. An unchanged failed selection retries
  only Playwright's recorded failed tests; setup failures without failed test IDs
  rerun the full selection. For repeated uncached browser runs, keep
  `pnpm test:e2e:server` running in another terminal; the runner reuses it only
  when its repository/configuration fingerprint matches.
  Browser cache keys include only the selected feature's source/import closure,
  changed inputs, and selected specs; unrelated UI edits do not invalidate them.
- UI unit tests selected by `pnpm validate:changed` follow relative imports and
  run in up to four balanced isolated shard processes. Full `pnpm test` uses the same concurrency.
  Test files are discovered from `packages/ui/src/**/*.test.ts(x)` rather than a
  manually maintained package script list.
  Selected UI tests are cached independently by their runtime import closure.
  Local UI runs stop scheduling new tests after the first failure; CI keeps
  collecting failures. Pass `--no-fail-fast` to the TSX runner when needed. The
  runner records local durations and balances historically slow tests across shards.
- Routine Rust source changes test and Clippy the directly changed crates, then
  `cargo check` their in-repository dependents. Cargo manifests, lockfiles, and
  toolchain configuration retain full-workspace test and Clippy validation.
  Successful Rust validation commands are content-cached locally; use
  `pnpm validate:changed --no-cache` to force execution.

Before completing a routine task, run `pnpm validate:changed`. Run the applicable
full validation once before merging to `main`, cutting a release, or after changing
shared manifests/toolchain configuration:

- TypeScript changes: `pnpm typecheck` and `pnpm test`.
- Rust changes: `cargo test --workspace --locked` and
  `cargo clippy --workspace --all-targets --locked -- -D warnings`.
- UI changes: attach screenshots and run the smallest relevant browser smoke test
  after the UI is stable. Run the complete `pnpm test:e2e` before merge.
- Documentation-only changes do not require code tests.
- If final full validation has passed and subsequent edits do not affect code,
  do not rerun it.

## Security rules

- Never commit credentials or certificates.
- Never log unmasked authorization headers.
- Treat import files and plugin packages as untrusted input.
- Do not weaken Local Agent authentication.
- Do not enable remote binding by default.
- Changes to secrets, proxying, scripts, and plugins require security review.
- Before merging security-, dependency-, workflow-, or parser-related changes, apply
  [docs/maintainers/PR_REVIEW_CHECKLIST.md](docs/maintainers/PR_REVIEW_CHECKLIST.md)
  so CI/CodeQL/Installer failures are caught in review rather than only on GitHub Actions.

## Scope rules

- Keep pull requests focused.
- Update documentation with behavior changes.
- Add regression tests for bug fixes.
- Preserve backward compatibility for workspace files where possible.

## UI icon rules

- Use Lucide through the shared `@protovoy/design-system` (re-exported by `@protovoy/ui`) icon abstraction for product UI;
  do not add another general-purpose icon library or page-local SVG copy.
- Use official SVG assets or Simple Icons only for third-party brand marks. Keep
  the ProtoVoy product mark in the repository-owned brand assets.
- Do not use emoji or font glyphs such as `•••`, `↑`, `×`, or `✓` as interactive
  control icons. Protocol payload or traffic-direction symbols may remain text
  when they are data rather than controls.
- Icon-only controls require an accessible name; decorative icons must be hidden
  from assistive technology.
- Follow the sizing, stroke, naming, exception, and migration rules in
  [docs/UI_UX_DESIGN_SYSTEM.md](docs/UI_UX_DESIGN_SYSTEM.md#6-图标系统).

See [docs/maintainers/CODEX_WORKFLOWS.md](docs/maintainers/CODEX_WORKFLOWS.md) for planned Codex maintainer workflows.
See [docs/maintainers/PR_REVIEW_CHECKLIST.md](docs/maintainers/PR_REVIEW_CHECKLIST.md) for front-loaded review checks.
