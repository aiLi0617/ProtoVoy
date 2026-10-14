# Install ProtoVoy

## Desktop (recommended)

Download the latest release for your platform from [GitHub Releases](https://github.com/aiLi0617/ProtoVoy/releases):

| Platform | Package |
|----------|---------|
| Windows | `.msi` installer |
| macOS | `.dmg` disk image |
| Linux | `.deb` package |

Verify downloads using the `SHA256SUMS.txt` file attached to each release.

## Local Agent (Web workbench)

The Web UI requires the Local Agent on the same machine:

```bash
# After installing or building protovoy-agent
protovoy-agent
```

Default listen address: `127.0.0.1:39217`. Pair from the Web UI on first connect.

## CLI

```bash
protovoy-cli drivers
protovoy-cli http-get https://httpbin.org/get
```

CLI binaries are published alongside Desktop releases.

## Build from source

See [Development](../../README.md#development) in the root README and [CONTRIBUTING.md](../../CONTRIBUTING.md).
