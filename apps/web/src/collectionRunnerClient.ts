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

/** Run orchestration belongs to Local Agent so navigation or tree refresh cannot split a run. */
export async function runCollectionViaAgent(
  collectionId: string,
  failFast: boolean,
): Promise<CollectionRunCase[]> {
  await checkAgentHandshake();
  const response = await fetch(
    `${agentBase()}/v1/collections/${encodeURIComponent(collectionId)}/runs`,
    {
      method: "POST",
      headers: agentRequestHeaders(),
      body: JSON.stringify({ failFast }),
    },
  );
  if (!response.ok) throw new Error(await response.text());
  return response.json() as Promise<CollectionRunCase[]>;
}
