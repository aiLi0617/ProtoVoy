export function shortcutModifier(platform?: string): "Ctrl" | "⌘" {
  const value = platform ?? (typeof navigator === "undefined" ? "" : navigator.platform);
  return /mac|iphone|ipad|ipod/i.test(value) ? "⌘" : "Ctrl";
}

export function shortcutLabel(key: string, platform?: string): string {
  return `${shortcutModifier(platform)} ${key}`;
}
