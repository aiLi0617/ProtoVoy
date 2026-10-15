import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";

const sources = ["GrpcWorkbench.tsx", "WebSocketWorkbench.tsx", "SseWorkbench.tsx", "SocketWorkbench.tsx"]
  .map((file) => [file, fs.readFileSync(path.resolve("src", file), "utf8")] as const);

for (const [file, source] of sources) {
  assert.match(source, /import \{ Button, TextInput \} from "\.\/Components";/, `${file} should use the shared command-bar primitives`);
  assert.match(source, /<TextInput className="websocket-name"/, `${file} should use the shared text input for the interface name`);
}

function commandBar(source: string, className: string): string {
  const match = source.match(new RegExp(`<div className="${className}">([\\s\\S]*?)</div>`));
  assert.ok(match, `expected ${className}`);
  return match[1];
}

assert.doesNotMatch(commandBar(sources[0][1], "grpc-commandbar"), /className="grpc-(?:primary|secondary|danger)"/, "gRPC command actions should not restore private button variants");
assert.doesNotMatch(commandBar(sources[1][1], "websocket-commandbar"), /className="websocket-(?:primary|secondary|danger)"/, "WebSocket command actions should not restore private button variants");
assert.doesNotMatch(commandBar(sources[2][1], "sse-target-row"), /className="ui-button (?:primary|secondary|danger)"/, "SSE should render the shared Button component instead of imitating its CSS contract");

assert.doesNotMatch(sources[3][1], /className="websocket-(?:primary|secondary|danger)"/, "TCP/UDP actions should use shared Button variants");

const historySource = fs.readFileSync(path.resolve("src", "ProtocolHistoryWorkbench.tsx"), "utf8");
assert.match(historySource, /import \{ Button, Textarea, TextInput \} from "\.\/Components";/);
assert.doesNotMatch(historySource, /className="ui-button /, "protocol history should not imitate shared primitive CSS");

const scriptsSource = fs.readFileSync(path.resolve("src", "ScriptLibraryWorkbench.tsx"), "utf8");
assert.match(scriptsSource, /import \{ Button, IconButton, Select, TextInput \} from "\.\/Components";/);
assert.doesNotMatch(scriptsSource, /className="ui-(?:button|icon-button)/, "script library should render shared primitives directly");

const httpSource = fs.readFileSync(path.resolve("src", "HttpWorkbench.tsx"), "utf8");
assert.match(httpSource, /import \{ Button, IconButton \} from "\.\/Components";/);
assert.doesNotMatch(httpSource, /className="ui-(?:button |icon-button)/, "HTTP actions should render shared button primitives directly");
assert.doesNotMatch(httpSource, /styles\.(?:button|secondaryButton)/, "HTTP actions should not restore private button styles");

const protocolLayoutSource = fs.readFileSync(path.resolve("src", "ProtocolWorkbenchLayout.tsx"), "utf8");
assert.match(protocolLayoutSource, /import \{ Button, TextInput \} from "\.\/Components";/);
assert.doesNotMatch(protocolLayoutSource, /<(?:button|input)[^>]*className="protocol-(?:run|save|cancel|name|target)-/, "shared protocol layout controls should use design-system primitives");

console.log("Workbench shared primitive tests passed");
