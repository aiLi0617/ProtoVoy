function preserveVariableTemplates(value: string): string {
  return value.replaceAll("%7B%7B", "{{").replaceAll("%7D%7D", "}}");
}

/** Returns unique single-brace placeholders from the URL path. */
export function pathParameterNames(url: string): string[] {
  const path = url.split(/[?#]/, 1)[0];
  const names: string[] = [];
  const seen = new Set<string>();
  const pattern = /\{([^{}\/]+)\}/g;
  for (const match of path.matchAll(pattern)) {
    const offset = match.index ?? 0;
    // Double braces belong to ProtoVoy's environment-variable syntax.
    if (path[offset - 1] === "{" || path[offset + match[0].length] === "}") continue;
    const name = match[1].trim();
    if (!name || seen.has(name)) continue;
    seen.add(name);
    names.push(name);
  }
  return names;
}

/** Replaces path placeholders for execution without touching the query or hash. */
export function resolvePathParameterUrl(url: string, values: ReadonlyMap<string, string>): string {
  const suffixIndex = url.search(/[?#]/);
  const path = suffixIndex >= 0 ? url.slice(0, suffixIndex) : url;
  const suffix = suffixIndex >= 0 ? url.slice(suffixIndex) : "";
  const resolvedPath = path.replace(/\{([^{}\/]+)\}/g, (placeholder, rawName: string, offset: number) => {
    if (path[offset - 1] === "{" || path[offset + placeholder.length] === "}") return placeholder;
    const value = values.get(rawName.trim());
    if (value === undefined) return placeholder;
    return preserveVariableTemplates(encodeURIComponent(value));
  });
  return `${resolvedPath}${suffix}`;
}
