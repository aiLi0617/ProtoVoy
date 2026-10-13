export function readStorage(primary: string): string | null {
  try {
    return localStorage.getItem(primary);
  } catch {
    return null;
  }
}

export type RuntimeAgentConfig = {
  agentUrl?: string;
  agentToken?: string;
};

const DEFAULT_AGENT_URL = "http://127.0.0.1:39217";

export function bootstrapClientStorage(): void {}

export function resolveAgentUrl(options?: {
  runtime?: RuntimeAgentConfig;
  envUrl?: string;
}): string {
  const override = readStorage("protovoy:agent-url")?.trim();
  if (override) return override;
  return options?.runtime?.agentUrl ?? options?.envUrl ?? DEFAULT_AGENT_URL;
}

export function resolveAgentToken(options?: {
  runtime?: RuntimeAgentConfig;
  envToken?: string;
}): string | undefined {
  const override = readStorage("protovoy:agent-token");
  if (override) return override;
  return options?.runtime?.agentToken ?? options?.envToken;
}

export function persistAgentToken(token: string): void {
  try {
    localStorage.setItem("protovoy:agent-token", token);
  } catch {
    /* ignore */
  }
}
