import type { RequestEnvelope } from "@protovoy/request-model";
import type { WorkspaceTree } from "@protovoy/ui";
import { agentBaseUrl, agentRequestHeaders, checkAgentHandshake } from "./agentClient";

export function describeWorkspaceBootstrapError(error: unknown, abortedReason?: unknown): string {
  if (abortedReason === "timeout") return "连接 Local Agent 超时（10 秒）。请确认 Agent 已启动，并检查设置中的地址。";
  const message = error instanceof Error ? error.message : String(error);
  if (/401|unauthorized|session exchange/i.test(message)) return "Local Agent 在线，但尚未完成配对。请在设置中填写 Agent Token 后重试。";
  if (/failed to fetch|networkerror|load failed|network request failed/i.test(message)) return "无法连接 Local Agent。请先启动 Agent，或检查设置中的地址与浏览器跨域配置。";
  if (/health check failed:\s*404/i.test(message)) return "已连接到目标地址，但它不是兼容的 Local Agent。请检查 Agent 地址。";
  if (/health check failed:\s*5\d\d/i.test(message)) return "Local Agent 暂时不可用。请查看 Agent 日志后重试。";
  return message || "无法加载工作区。请检查 Local Agent 设置后重试。";
}

/** Abort-aware workspace bootstrap. The preliminary health request also bounds offline Agent waits. */
export async function getWorkspaceTreeAbortable(signal: AbortSignal): Promise<WorkspaceTree> {
  const health = await fetch(`${agentBaseUrl}/health`, { signal });
  if (!health.ok) throw new Error(`Agent health check failed: ${health.status}`);
  await checkAgentHandshake();
  if (signal.aborted) throw signal.reason;
  const response = await fetch(`${agentBaseUrl}/v1/workspace-tree`, { headers: agentRequestHeaders(), signal });
  if (!response.ok) throw new Error(await response.text());
  const raw = await response.json() as Omit<WorkspaceTree, "requests"> & { requests: Array<WorkspaceTree["requests"][number] & { envelope?: RequestEnvelope }> };
  return { ...raw, requests: raw.requests.map((item) => ({
    ...item,
    method: item.envelope?.payload.type === "http" ? item.envelope.payload.method : item.method,
  })) };
}
