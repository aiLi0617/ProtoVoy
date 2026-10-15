import { agentBaseUrl, agentRequestHeaders, checkAgentHandshake } from "./agentClient";

type GlobalParameter = { location: "header" | "cookie" | "query" | "body"; name: string; value: string; enabled: boolean; description: string };
interface EnvironmentConfig { variables: Record<string, string>; globalParameters?: GlobalParameter[]; secretRefs?: string[] }

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  await checkAgentHandshake();
  const response = await fetch(`${agentBaseUrl}${path}`, { ...init, headers: agentRequestHeaders(init?.headers) });
  if (!response.ok) throw new Error(await response.text());
  return response.status === 204 ? undefined as T : response.json();
}

export async function getEnvironmentConfig(): Promise<EnvironmentConfig> {
  const environment = await request<EnvironmentConfig>("/v1/environments/default");
  return { variables: environment.variables ?? {}, globalParameters: environment.globalParameters ?? [], secretRefs: environment.secretRefs ?? [] };
}

export async function saveEnvironmentConfig(variables: Record<string, string>, secretRefs: string[], globalParameters: GlobalParameter[]): Promise<void> {
  await request("/v1/environments/default", { method: "PUT", body: JSON.stringify({ variables, secretRefs, globalParameters }) });
}

export async function secretExists(name: string): Promise<boolean> {
  return (await request<{ exists: boolean }>(`/v1/secrets/status?name=${encodeURIComponent(name)}`)).exists;
}

export async function deleteSecret(name: string): Promise<void> {
  await request("/v1/secrets", { method: "DELETE", body: JSON.stringify({ name }) });
}
