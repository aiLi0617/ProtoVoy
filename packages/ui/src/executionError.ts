const NETWORK_ERROR_PATTERNS = [
  /failed to fetch/i,
  /network\s*error/i,
  /load failed/i,
  /connection (?:refused|reset)/i,
  /econn(?:refused|reset)/i,
];

function rawErrorMessage(error: unknown): string {
  if (error instanceof Error) return error.message.trim();
  return String(error ?? "").trim();
}

export function describeExecutionError(error: unknown, action = "操作"): string {
  const message = rawErrorMessage(error);
  if (!message) return `${action}失败，请稍后重试。`;
  if (NETWORK_ERROR_PATTERNS.some((pattern) => pattern.test(message))) {
    return `${action}失败：无法连接执行服务。请确认 Local Agent 或桌面后端已启动，并检查连接设置。`;
  }
  if (/\b(?:timeout|timed out)\b/i.test(message)) {
    return `${action}超时。请检查目标地址与网络连接后重试。`;
  }
  if (/\b(?:401|unauthorized)\b/i.test(message)) {
    return `${action}失败：身份验证已失效，请检查连接令牌。`;
  }
  if (/\b(?:403|forbidden)\b/i.test(message)) {
    return `${action}失败：当前连接没有所需权限。`;
  }
  return `${action}失败：${message}`;
}
