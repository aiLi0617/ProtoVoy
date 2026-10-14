import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import ts from "typescript";

const roots = [path.resolve("src"), path.resolve("../../apps/web/src"), path.resolve("../../apps/desktop/src")];
const files: string[] = [];
const invalidControls: string[] = [];
const forbiddenGlyph = /(?:•••|⋯|×|↑|↓|✓)/u;

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
    if (ts.isJsxElement(node) && node.openingElement.tagName.getText(source) === "button") {
      const directLabel = node.children.map((child) => {
        if (ts.isJsxText(child)) return child.text;
        if (ts.isJsxExpression(child) && child.expression && ts.isStringLiteral(child.expression)) return child.expression.text;
        return "";
      }).join("");
      if (forbiddenGlyph.test(directLabel)) {
        const position = source.getLineAndCharacterOfPosition(node.getStart(source));
        invalidControls.push(`${path.relative(process.cwd(), file)}:${position.line + 1}`);
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
}

assert.deepEqual(invalidControls, [], `Interactive controls must use shared icons instead of font glyphs:\n${invalidControls.join("\n")}`);
console.log("Control icon glyph tests passed");
