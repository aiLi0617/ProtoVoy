export const GLOBAL_VARIABLES_ENVIRONMENT_ID = "default-env";
export const LOCAL_ENVIRONMENT_ID = "local-env";
export const LOCAL_ENVIRONMENT_NAME = "\u672c\u5730\u73af\u5883";

export function environmentOptionLabel(
  environment: { id: string; name: string },
  _defaultEnvironmentId = LOCAL_ENVIRONMENT_ID,
): string {
  return environment.id === LOCAL_ENVIRONMENT_ID
    ? LOCAL_ENVIRONMENT_NAME
    : environment.name.trim() || "\u672a\u547d\u540d\u73af\u5883";
}
