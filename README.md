# ProtoVoy

**Explore Every Protocol.**

[![CI](https://github.com/aiLi0617/ProtoVoy/actions/workflows/ci.yml/badge.svg)](https://github.com/aiLi0617/ProtoVoy/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/aiLi0617/ProtoVoy?include_prereleases)](https://github.com/aiLi0617/ProtoVoy/releases)
[![License](https://img.shields.io/github/license/aiLi0617/ProtoVoy)](LICENSE)
[![Issues](https://img.shields.io/github/issues/aiLi0617/ProtoVoy)](https://github.com/aiLi0617/ProtoVoy/issues)

ProtoVoy is a lightweight, local-first and extensible multi-protocol API debugging client for Windows, macOS, Linux and the Web.

ProtoVoy provides a unified workspace for debugging HTTP, GraphQL, gRPC, WebSocket, SSE, TCP/UDP, MQTT, AMQP, Kafka, Redis and SQL protocols.

ProtoVoy 是一款轻量、本地优先、可扩展的多协议接口调试工具。

## Preview

![ProtoVoy overview](docs/images/overview.png)

## Why ProtoVoy?

A local-first, extensible workbench for API and infrastructure protocols.

- **Unified workspace** — one client for HTTP APIs and infrastructure protocols
- **Local-first** — SQLite storage, OS keychain secrets, no account required
- **Shared core** — Desktop, Web, CLI and Local Agent reuse the same Rust drivers
- **Extensible** — QuickJS scripts and WASM plugins (experimental)
- **Open source** — all code in this repository is Apache-2.0

## Features

- Multi-protocol request editor with streaming responses and execution timeline
- Environments, variables, assertions, and request history
- Import from cURL, OpenAPI, HAR, and Postman
- Code generation for HTTP and several protocol workbenches
- Collection execution in CLI with CI-friendly exit codes; the UI workflow will be consolidated into automation testing
- OS keychain-backed secret storage
- Optional traffic capture proxy (loopback defaults, sensitive header masking)
- Optional self-hosted collaboration and private deployment (preview)

## Supported protocols

| Protocol | Execute | Streaming | History | Code generation |
|----------|:-------:|:---------:|:-------:|:---------------:|
| HTTP/HTTPS | ✅ | ✅ | ✅ | ✅ |
| GraphQL | ✅ | ✅ | ✅ | ✅ |
| gRPC | Experimental | Experimental | ✅ | Experimental |
| WebSocket | ✅ | ✅ | ✅ | ✅ |
| SSE | ✅ | ✅ | ✅ | ✅ |
| TCP/UDP | ✅ | ✅ | ✅ | ✅ |
| MQTT | ✅ | ✅ | ✅ | Planned |
| AMQP | Experimental | Experimental | ✅ | Planned |
| Kafka | Experimental | Experimental | ✅ | Planned |
| Redis | ✅ | — | ✅ | Planned |
| SQL | ✅ | — | ✅ | Planned |

Legend: ✅ = Beta (verified by tests); **Experimental** = implemented but limited verification; **Planned** = not yet available; **—** = not applicable.

GraphQL is available via the HTTP workbench GraphQL body mode and a dedicated driver. See [examples/](examples/) and [docs/protocols/](docs/protocols/) for reproducible samples.

## Quick start

### Run the Viva workbench

The [Viva bundles workflow](.github/workflows/viva-bundle.yml) builds
`protovoy-viva-<tag>-<platform>.tar.gz` for Windows x64, Linux x64 and macOS arm64.
Look for the archive and `SHA256SUMS.txt` in releases produced by that workflow.
Extract it and follow the included README to start the desktop workbench,
Web server, CLI or Local Agent.

### Tauri installers

Releases are built as GitHub Drafts first, then published after maintainer verification.

Current release target: [v0.1.0](https://github.com/aiLi0617/ProtoVoy/releases/tag/v0.1.0). Draft assets include:

- **Windows**: `.msi`
- **macOS**: `.dmg`
- **Linux**: `.deb`
- **ProtoVoy CLI** and **ProtoVoy Local Agent** archives
- `SHA256SUMS.txt` for checksum verification

### Send your first request

1. Create a workspace.
2. Select **HTTP**.
3. Enter `https://httpbin.org/get`.
4. Click **Send**.
5. Inspect the response, timeline, and assertions.

For the Web workbench, start the Local Agent (`protovoy-agent`) and pair it from the Web UI.

## Development

### Viva + Go workbench

Use Go 1.26 or newer. Run these commands from the repository root.

On Windows, `.\scripts\start.ps1` starts the Viva desktop workbench.
Use `.\scripts\start.ps1 -Mode Release` to build the Go executables into
`apps/protovoy-go/build/native` and start the compiled workbench.
Use `.\scripts\start.ps1 -Surface Web` to build and serve the browser
workbench at `http://127.0.0.1:39300/`. Add `-Mode Release` to run the
compiled Go server.

```sh
# Desktop workbench
go -C apps/protovoy-go run ./cmd/workbench

# Browser workbench
go -C apps/protovoy-go tool viva build --target web -o build/web
go -C apps/protovoy-go run ./cmd/server -target http://127.0.0.1:8080/
```

For the browser example, start your HTTP service on port 8080 and open
`http://127.0.0.1:39300/`. The Go server forwards requests to the configured
target. See the [Viva + Go guide](apps/protovoy-go/README.md) for the CLI,
Local Agent pairing, and [HTTPS deployment](apps/protovoy-go/README.md#docker-compose-deployment).

The root `pnpm dev:desktop` and `pnpm dev:web` scripts also start Viva.
The Web script serves the workbench on loopback. Pair a Go Local Agent
or pass the server a fixed execution target.

### Requirements

The existing TypeScript, Rust and Java applications use the following tools.

- Node.js 22+
- pnpm 10.27+
- Rust stable
- Platform-specific [Tauri dependencies](https://v2.tauri.app/start/prerequisites/)
- Java 21 (only for `services/collaboration-server/`)

```bash
git clone https://github.com/aiLi0617/ProtoVoy.git
cd ProtoVoy
pnpm install
cargo check --workspace
pnpm dev:desktop:tauri
```

On Windows, use the Tauri option of the bootstrap script to develop that
desktop application:

```powershell
powershell -NoProfile -File .\scripts\start.ps1 -Stack Tauri
```

To build the release application (including the Local Agent sidecar and the
Tauri installer) and then launch the release executable:

```powershell
powershell -NoProfile -File .\scripts\start.ps1 -Stack Tauri -Mode Release
```

Pass `-SkipInstall` on later runs when the dependencies are already installed.

See [CONTRIBUTING.md](CONTRIBUTING.md) for validation commands and pull request expectations.

## Downloads

The Viva bundle includes `workbench` or `ProtoVoy.app`, `protovoy`, `agent`,
`server` and the browser assets under `web/`.

The Tauri and Rust artifacts use these executable names.

| Artifact | Binary | Description |
|----------|--------|-------------|
| Desktop | `protovoy` | Tauri desktop app |
| Local Agent | `protovoy-agent` | Headless protocol runtime for Web |
| CLI | `protovoy-cli` | CI and terminal automation |

Release assets include platform installers, CLI/Agent archives, and SHA-256 checksums.

## Security and privacy

- Secrets are stored in the OS keychain (Desktop) or Agent secret store; projects keep `secret_ref` only.
- Local Agent binds to loopback by default with Bearer token authentication.
- Do **not** report security vulnerabilities via public issues — see [SECURITY.md](SECURITY.md).

## Architecture

```text
Desktop / Web UI  →  Local Agent or Tauri commands  →  ExecutionEngine  →  Protocol drivers
CLI               →  ExecutionEngine (in-process)   →  Protocol drivers
```

Desktop and Local Agent use the same per-user **SQLite + local blob files + OS keychain** data directory by default, so Desktop and a locally paired Web workbench see the same persisted workspace data. Set `PROTOVOY_CONFIG_DIR` before starting both processes to use an isolated profile. Optional collaboration server and protocol gateway support self-hosted deployment.

Details: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) · [docs/PORTS.md](docs/PORTS.md) · [docs/THREAT_MODEL.md](docs/THREAT_MODEL.md) · [docs/ISSUES.md](docs/ISSUES.md) · [docs/UX_BACKLOG.md](docs/UX_BACKLOG.md)

## Comparison

| Capability | ProtoVoy |
|------------|--------|
| Local-first | ✅ |
| Open-source source code | ✅ |
| Desktop + Web + CLI | ✅ |
| HTTP / GraphQL / gRPC | ✅ |
| TCP/UDP / MQ / Redis / SQL | ✅ |
| WASM plugins | Experimental |
| Git-friendly workspace | ✅ |

## Roadmap

| Milestone | Focus |
|-----------|--------|
| **v0.1 Public Alpha** | First public release, core protocols, installers |
| **v0.2 Protocol Stability** | gRPC/MQ broker tests, compatibility matrix |
| **v0.3 Plugin Ecosystem** | Example WASM plugins, publisher docs |
| **v1.0 Stable** | i18n completion, auto-update, hardened defaults |

Track work on [GitHub Milestones](https://github.com/aiLi0617/ProtoVoy/milestones).

## Contributing

We welcome issues and pull requests. Please read [CONTRIBUTING.md](CONTRIBUTING.md), [CHANGELOG.md](CHANGELOG.md), and [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

Community discussion: [GitHub Discussions](https://github.com/aiLi0617/ProtoVoy/discussions).

## Open-source scope

All source code contained in this repository is licensed under the Apache License 2.0, except for the ProtoVoy trademarks and brand assets described in [NOTICE](NOTICE).

ProtoVoy may offer separately developed hosted services in the future. Those services are not part of this repository.

Resources received through the Codex for Open Source program will be used exclusively for maintaining this public repository and its open-source workflows.

## License

Licensed under the [Apache License 2.0](LICENSE). ProtoVoy name, logo, and slogan are not licensed under Apache-2.0 — see [NOTICE](NOTICE).
