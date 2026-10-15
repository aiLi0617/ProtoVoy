import { resolveAgentUrl } from "@protovoy/app-wiring";
import type { CollectionRunCase } from "@protovoy/ui";
import { agentRequestHeaders, checkAgentHandshake } from "./agentClient";

const runtime = (window as Window & {
  __PROTOVOY_CONFIG__?: { agentUrl?: string };
}).__PROTOVOY_CONFIG__;

function agentBase(): string {
  return resolveAgentUrl({
    runtime,
    envUrl: import.meta.env.VITE_PROTOVOY_AGENT_URL as string | undefined,
  });
}

export async function listCollectionEnvironments(
  projectId: string,
): Promise<Array<{ id: string; name: string }>> {
  await checkAgentHandshake();
  const response = await fetch(
    `${agentBase()}/v1/environments?projectId=${encodeURIComponent(projectId)}`,
    { headers: agentRequestHeaders() },
  );
  if (!response.ok) throw new Error(await response.text());
  const records = (await response.json()) as Array<{ id: string; name: string }>;
  return records.map(({ id, name }) => ({ id, name }));
}

/** Keep collection orchestration in Local Agent so navigation cannot split a run. */
export async function runCollection(
  collectionId: string,
  failFast: boolean,
  environmentId?: string,
): Promise<CollectionRunCase[]> {
  await checkAgentHandshake();
  const response = await fetch(
    `${agentBase()}/v1/collections/${encodeURIComponent(collectionId)}/runs`,
    {
      method: "POST",
      headers: agentRequestHeaders(),
      body: JSON.stringify({ failFast, ...(environmentId ? { environmentId } : {}) }),
    },
  );
  if (!response.ok) throw new Error(await response.text());
  return response.json() as Promise<CollectionRunCase[]>;
}
