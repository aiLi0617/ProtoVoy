export const LOCAL_ENVIRONMENT_ID = "default-env";
export const LOCAL_ENVIRONMENT_NAME = "本地环境";

export function environmentOptionLabel(
  environment: { id: string; name: string },
  defaultEnvironmentId = LOCAL_ENVIRONMENT_ID,
): string {
  return environment.id === defaultEnvironmentId
    ? `${LOCAL_ENVIRONMENT_NAME} · 默认`
    : environment.name.trim() || "未命名环境";
}
