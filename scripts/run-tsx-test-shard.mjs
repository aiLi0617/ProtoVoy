import { resolve } from "node:path";
import { pathToFileURL } from "node:url";

const files = process.argv.slice(2).filter((argument) => argument !== "--split-component-tests");
for (const file of files) {
  await import(pathToFileURL(resolve(process.cwd(), file)).href);
}
