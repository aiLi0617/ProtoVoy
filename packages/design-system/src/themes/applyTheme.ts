export type ThemeMode = "dark" | "light" | "system";
export type ResolvedTheme = "dark" | "light";

export function resolveSystemTheme(getMatches?: () => boolean): ResolvedTheme {
  if (typeof window === "undefined") return "dark";
  const matches = getMatches ?? (() => window.matchMedia("(prefers-color-scheme: dark)").matches);
  return matches() ? "dark" : "light";
}

export function resolveTheme(mode: ThemeMode): ResolvedTheme {
  return mode === "system" ? resolveSystemTheme() : mode;
}

/** Apply theme to documentElement via data-theme. Safe for SSR (no-op). */
export function applyTheme(mode: ThemeMode, root?: HTMLElement): ResolvedTheme {
  const resolved = resolveTheme(mode);
  if (typeof document === "undefined" && !root) return resolved;
  const el = root ?? document.documentElement;
  el.dataset.theme = resolved;
  return resolved;
}
