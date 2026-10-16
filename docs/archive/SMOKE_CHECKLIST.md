
> **Archived snapshot — not SoT.** See [README.md](./README.md).

# 闃舵 0 楠屾敹鐑熸祴娓呭崟

瀵圭収 [`PHASE_PLAN.md`](./PHASE_PLAN.md) 搂5 / 搂12銆?

## CLI

```bash
cargo run -p protovoy-cli -- http-get https://example.com
cargo run -p protovoy-cli -- drivers
```

鏈熸湜锛氭墦鍗?`ExecutionSummary` JSON銆乣executionId`銆乪vents 璁℃暟锛沗drivers` 鍚?`http`銆?

## Local Agent + 姝ｅ紡鎵ц API

```bash
# 缁堢 1
cargo run -p protovoy-local-agent

# 缁堢 2锛堝皢 TOKEN 鎹㈡垚 Agent 鎵撳嵃鐨勯厤瀵逛护鐗岋級
curl -s http://127.0.0.1:39217/health
curl -s -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -H "X-ProtoVoy-Protocol-Api-Version: 1" \
  -H "X-ProtoVoy-Client: cli-smoke" \
  -H "X-ProtoVoy-Client-Version: 0.1.0" \
  -d "{\"id\":\"00000000-0000-4000-8000-000000000001\",\"protocolId\":\"http\",\"name\":\"smoke\",\"target\":\"https://example.com\",\"environmentRef\":null,\"authRef\":null,\"timeoutMs\":30000,\"retryPolicy\":{\"max_retries\":0,\"backoff_ms\":0},\"proxy\":null,\"tls\":{\"verify\":true,\"client_cert_ref\":null},\"metadata\":{},\"payload\":{\"type\":\"http\",\"method\":\"GET\",\"headers\":[],\"body\":null,\"followRedirects\":true},\"preScripts\":[],\"postScripts\":[],\"assertions\":[],\"createdAt\":\"2026-08-05T00:00:00Z\"}" \
  http://127.0.0.1:39217/v1/executions
```

鏈熸湜锛歚/health` JSON 鍚?`"service":"protovoy-agent"`锛堟敞鎰忥細crate 鍚嶆槸 `protovoy-local-agent`锛屼簩杩涘埗鏄?`protovoy-agent`锛宧ealth 瀛楁鐢ㄥ悗鑰咃級銆乣protocolApiVersion` / `agentVersion`锛沗POST /v1/executions` 杩斿洖 `202` + `executionId`銆傞殢鍚庯細

```bash
curl -N -H "Authorization: Bearer $TOKEN" \
  -H "X-ProtoVoy-Protocol-Api-Version: 1" \
  http://127.0.0.1:39217/v1/executions/<executionId>/events
```

鏈熸湜锛歋SE `event: execution` 娴侊紝鍚?`response_meta` / `completed`銆?

鍗忚鐗堟湰涓嶅吋瀹规椂锛堝簲 426锛夛細

```bash
curl -s -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -H "X-ProtoVoy-Protocol-Api-Version: 999" \
  -d "{...鍚屼笂...}" \
  http://127.0.0.1:39217/v1/executions
```

## Web + Agent

```bash
# 璁剧疆 VITE_PROTOVOY_AGENT_TOKEN 鍚?
pnpm --filter @protovoy/web dev
```

鏈熸湜锛氬伐浣滃彴鍙敼 method / headers / body / timeout锛涘彂閫佽蛋姝ｅ紡 API锛涘彇娑堟寜閽湪鎵ц涓彲鐢ㄣ€?

## Desktop

```bash
pnpm --filter @protovoy/desktop tauri dev
# 鎴?
pnpm dev:desktop
```

鏈熸湜锛氬彂閫?HTTP锛涗繚瀛樿姹傚悗銆屾墦寮€鏈€杩戣姹傘€嶅彲鎭㈠锛沗version_info` 鍗忚鐗堟湰涓?`1`銆?

## SQLite 鍗曟祴

```bash
cargo test -p local-store
```
