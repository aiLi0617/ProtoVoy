export interface ProtoVoyRuntimeConfig { agentUrl?: string; agentToken?: string; collaborationUrl?: string }

export function runtimeConfig(): ProtoVoyRuntimeConfig {
  const win = window as Window & {
    __PROTOVOY_CONFIG__?: ProtoVoyRuntimeConfig;
  };
  return win.__PROTOVOY_CONFIG__ ?? {};
}

export function defaultCollaborationUrl(): string {
  return runtimeConfig().collaborationUrl ?? "http://localhost:8088";
}
