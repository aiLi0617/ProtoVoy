# Default bind ports

Canonical loopback defaults for local development. Compose and private deploy may override to container interfaces; see `deploy/`.

| Service | Default bind | Env override |
|---------|--------------|--------------|
| Local Agent | `127.0.0.1:39217` | `PROTOVOY_AGENT_BIND` |
| Mock Server | `127.0.0.1:39218` | Mock UI / Agent mock spawn |
| Protocol Gateway | `127.0.0.1:39219` | `PROTOVOY_GATEWAY_BIND` |
| Traffic Capture proxy | `127.0.0.1:39220` | Capture UI / Agent `start_capture` bind |
| Collaboration Server | `8088` (app) | `PROTOVOY_COLLAB_PORT` |
| Web / Vite | `5180` | Vite config |

Notes:

- Mock, Gateway, and Capture must not share default ports.
- Standalone binaries default to loopback. Remote bind is opt-in and must stay aligned with `AGENTS.md` / `THREAT_MODEL.md`.
- OAuth redirect examples that target the gateway use `:39219`.
