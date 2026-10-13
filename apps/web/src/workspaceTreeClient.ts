import type { RequestEnvelope } from "@protovoy/request-model";
import type { WorkspaceTree } from "@protovoy/ui";
import { agentBaseUrl, agentRequestHeaders, checkAgentHandshake } from "./agentClient";

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
