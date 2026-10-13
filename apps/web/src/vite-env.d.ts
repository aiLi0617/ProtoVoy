/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_PROTOVOY_AGENT_URL?: string;
  readonly VITE_PROTOVOY_AGENT_TOKEN?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
