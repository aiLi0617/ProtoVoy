export const smokeTags = new Set(["@app-shell", "@http", "@protocol-workbenches", "@settings"]);

export function validateSmokeTags(tags) {
  for (const tag of tags) {
    if (!smokeTags.has(tag)) throw new Error(`Unknown E2E smoke tag: ${tag}`);
  }
  return tags;
}
