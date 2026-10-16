import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import ts from "typescript";

const roots = [path.resolve("src"), path.resolve("../../apps/web/src"), path.resolve("../../apps/desktop/src")];
const files: string[] = [];
const missing: string[] = [];

function walk(directory: string) {
  for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
    const fullPath = path.join(directory, entry.name);
    if (entry.isDirectory()) walk(fullPath);
    else if (entry.name.endsWith(".tsx")) files.push(fullPath);
  }
}

for (const root of roots) walk(root);
for (const file of files) {
  const sourceText = fs.readFileSync(file, "utf8");
  const source = ts.createSourceFile(file, sourceText, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  const visit = (node: ts.Node) => {
    if ((ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) && node.tagName.getText(source) === "button") {
      const hasType = node.attributes.properties.some((property) => ts.isJsxAttribute(property) && property.name.getText(source) === "type");
      if (!hasType) {
        const position = source.getLineAndCharacterOfPosition(node.getStart(source));
        missing.push(`${path.relative(process.cwd(), file)}:${position.line + 1}`);
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
}

assert.deepEqual(missing, [], `Native buttons must declare type explicitly:\n${missing.join("\n")}`);
console.log("Native button type tests passed");
