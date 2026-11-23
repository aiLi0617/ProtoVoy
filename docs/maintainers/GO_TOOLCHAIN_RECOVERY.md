# Go toolchain validation recovery

The Viva + Go rewrite is incomplete. On 2026-11-23 the toolchain ran again:
formatting and `node scripts/validate-changed.mjs --go` passed, including the
OAuth Agent/CLI regression tests and Viva WebAssembly compilation. The diagnostic
record below is retained in case the execution-policy problem recurs.

## Observed blocker

Previously, application-control policy on the Windows development machine prevented
`D:\Work\Tool\Go\bin\gofmt.exe` and
`D:\Work\Tool\Go\pkg\tool\windows_amd64\compile.exe` from starting.
This is an operating-system execution restriction, not a failed project test or
a Codex workspace-write permission denial. Retrying with command execution
approval alone did not resolve it at that time.

The read-only diagnostic source is Event Viewer:

`Applications and Services Logs > Microsoft > Windows > CodeIntegrity > Operational`

Events **3077** and **3033** report that `gofmt.exe`, launched by the Codex
runtime's PowerShell process, does not meet enterprise signing requirements or
violates code-integrity policy. The observed policy ID is
`{0283ac0f-fff1-49ae-ada1-8a933130cad6}`.

A machine administrator must review the policy and permit a trusted official
Go toolchain, or provide a toolchain that meets the policy. No system security
policy was changed by this task. Do not treat cached tests or older executables
as validation of the latest source changes.

## Resume validation

From the repository root, once execution is permitted:

```powershell
gofmt -w apps/protovoy-go/cmd/protovoy/secrets.go apps/protovoy-go/cmd/protovoy/secrets_test.go apps/protovoy-go/cmd/agent/envelope.go apps/protovoy-go/cmd/agent/http_auth_test.go apps/protovoy-go/internal/secrets/http.go
pnpm validate:changed --go
```

The JavaScript validation entry point can also be invoked directly:

```powershell
node scripts/validate-changed.mjs --go
```

Use the existing repository Go caches when network access is unavailable; do not
change dependency versions to work around an execution-policy failure.

The regression cases covered by the successful validation include:

- `TestOAuthBindingCommandAndCollectionExecution`: bounded CLI input, real
  token exchange, resource header injection and credential-free project export.
- `TestAgentOAuthExchangeRunsInsideCancellableJob`: token resolution inside a
  registered job and cancellation before the resource request.
- Existing static HTTP binding, request preparation, OAuth binding and OAuth
  exchange tests, to detect regressions from deferred credential resolution.
- The validation runner's Viva WebAssembly compilation check.

The temporary unverified notes in `apps/protovoy-go/README.md` have been updated.
Remaining implementation includes Viva OAuth
configuration controls and the other open items in that document's rewrite
comparison table. Passing these focused checks does not complete the overall
rewrite or prove Docker deployment, remote CI or enterprise IdP integration.
