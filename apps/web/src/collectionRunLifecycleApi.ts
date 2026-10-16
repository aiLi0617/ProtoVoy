import { resolveAgentUrl } from "@protovoy/app-wiring";
import type { CollectionRunCase, CollectionRunProgress } from "@protovoy/ui";
import { agentRequestHeaders, checkAgentHandshake } from "./agentClient";

const runtime = (window as Window & { __PROTOVOY_CONFIG__?: { agentUrl?: string } }).__PROTOVOY_CONFIG__;

function agentBase(): string {
  return resolveAgentUrl({
    runtime,
    envUrl: import.meta.env.VITE_PROTOVOY_AGENT_URL as string | undefined,
  });
}

export async function runCollectionWithLifecycle(
  collectionId: string,
  failFast: boolean,
  environmentId: string | undefined,
  runId: string,
): Promise<CollectionRunCase[]> {
  await checkAgentHandshake();
  const response = await fetch(`${agentBase()}/v1/collections/${encodeURIComponent(collectionId)}/runs`, {
    method: "POST",
    headers: agentRequestHeaders(),
    body: JSON.stringify({ failFast, runId, ...(environmentId ? { environmentId } : {}) }),
  });
  if (!response.ok) throw new Error(await response.text());
  return response.json() as Promise<CollectionRunCase[]>;
}

export async function getCollectionRunProgress(runId: string): Promise<CollectionRunProgress> {
  await checkAgentHandshake();
  const response = await fetch(`${agentBase()}/v1/collection-runs/${encodeURIComponent(runId)}`, { headers: agentRequestHeaders() });
  if (!response.ok) throw new Error(await response.text());
  return response.json() as Promise<CollectionRunProgress>;
}

export async function cancelCollectionRun(runId: string): Promise<void> {
  await checkAgentHandshake();
  const response = await fetch(`${agentBase()}/v1/collection-runs/${encodeURIComponent(runId)}/cancel`, {
    method: "POST",
    headers: agentRequestHeaders(),
  });
  if (!response.ok) throw new Error(await response.text());
}
