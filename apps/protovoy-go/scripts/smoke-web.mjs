import assert from 'node:assert/strict';
import { execFile, spawn } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { once } from 'node:events';
import { mkdtemp, readFile, writeFile } from 'node:fs/promises';
import { createServer } from 'node:http';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { promisify } from 'node:util';

const chrome = process.env.CHROME_BIN || 'google-chrome';
const bundle = resolve(process.argv[2] || 'build/native');
const binary = name => join(bundle, name + (process.platform === 'win32' ? '.exe' : ''));
const port = process.env.PROTOVOY_SMOKE_PORT || '39300';
const origin = 'http://127.0.0.1:' + port;
const profile = await mkdtemp(join(tmpdir(), 'protovoy-viva-smoke-'));
const received = [];
const boundCredential = 'Bearer ' + randomBytes(24).toString('hex');
let boundCredentialReceived = false;
const openAPIResponse = '{"fixture":"openapi ok!"}';
const upstream = createServer(async (req, res) => {
  boundCredentialReceived ||= req.headers.authorization === boundCredential;
  const chunks = [];
  for await (const chunk of req) chunks.push(chunk);
  const body = Buffer.concat(chunks).toString();
  received.push({ method: req.method, path: req.url, body });
  res.writeHead(404, {
    'X-Fixture': ['first', 'second'],
    'Set-Cookie': 'session=fixture-cookie-value',
  }).end(body.includes('openapi fixture') ? openAPIResponse : 'upstream fixture response');
});
upstream.listen(0, '127.0.0.1');
await once(upstream, 'listening');

const target = 'http://127.0.0.1:' + upstream.address().port + '/upload';
const additionalTarget = new URL('/additional?items=1,2', target).href;
const app = spawn(binary('server'), ['-port', port, '-web', join(bundle, 'web'), '-target', target], {
  stdio: 'inherit', windowsHide: true,
  env: { ...process.env, PROTOVOY_HTTP_ALLOWED_TARGETS: additionalTarget },
});
const agentAddress = '127.0.0.1:' + (process.env.PROTOVOY_SMOKE_AGENT_PORT || '39316');
const agentToken = randomBytes(24).toString('hex');
const secretReference = 'browser-integration-' + randomBytes(12).toString('hex');
const agentOrigins = port === '39300' ? [] : ['-allowed-origins', origin];
const agent = spawn(binary('agent'), ['-bind', agentAddress, ...agentOrigins,
  '-history-dir', join(profile, 'agent-history')], {
  stdio: 'inherit', windowsHide: true, env: { ...process.env, PROTOVOY_AGENT_TOKEN: agentToken },
});
const browser = spawn(chrome, [
  '--headless', '--disable-background-networking', '--no-first-run', '--disable-extensions',
  '--remote-debugging-port=0', '--user-data-dir=' + profile, 'about:blank',
], { stdio: 'ignore', windowsHide: true });
for (const child of [app, browser, agent]) {
  child.on('error', error => console.error(error.message));
}

async function until(fn) {
  for (let attempt = 0; attempt < 150; attempt++) {
    try {
      const value = await fn();
      if (value) return value;
    } catch {}
    await delay(100);
  }
  throw new Error('Timed out waiting for ' + fn.toString().slice(0, 160));
}

let socket;
try {
  const mockRules = join(profile, 'mock-cli-fixture.json');
  const runMock = promisify(execFile);
  await runMock(binary('mock'), ['-rules', mockRules, '-init']);
  await runMock(binary('mock'), ['-rules', mockRules, '-check']);
  const initialRules = await readFile(mockRules, 'utf8');
  await assert.rejects(runMock(binary('mock'), ['-rules', mockRules, '-init']));
  assert.equal(await readFile(mockRules, 'utf8'), initialRules);
  await writeFile(mockRules, '[{"id":"invalid","method":"GET","status":200,"unknownField":true}]');
  await assert.rejects(runMock(binary('mock'), ['-rules', mockRules, '-check']));
  const port = await until(async () => (await readFile(resolve(profile, 'DevToolsActivePort'), 'utf8')).split('\n')[0]);
  await until(async () => (await fetch(origin)).ok);
  const pages = await (await fetch('http://127.0.0.1:' + port + '/json/list')).json();
  socket = new WebSocket(pages.find(page => page.type === 'page').webSocketDebuggerUrl);
  await once(socket, 'open');
  let next = 0;
  const requests = new Map();
  socket.addEventListener('message', ({ data }) => {
    const message = JSON.parse(data);
    if (!requests.has(message.id)) return;
    const { resolve, reject } = requests.get(message.id);
    requests.delete(message.id);
    if (message.error) reject(new Error(message.error.message));
    else resolve(message.result);
  });
  function send(method, params = {}) {
    const id = ++next;
    return new Promise((resolve, reject) => {
      requests.set(id, { resolve, reject });
      socket.send(JSON.stringify({ id, method, params }));
    });
  }
  async function value(expression) {
    return (await send('Runtime.evaluate', { expression, returnByValue: true })).result.value;
  }
  await send('Page.enable');
  await send('Emulation.setDeviceMetricsOverride',{width:1200,height:950,deviceScaleFactor:1,mobile:false});
  await send('Browser.setDownloadBehavior',{behavior:'allow',downloadPath:profile});
  await send('Page.addScriptToEvaluateOnNewDocument', { source: `
    window.drawnText = {};
    const fillText = CanvasRenderingContext2D.prototype.fillText;
    CanvasRenderingContext2D.prototype.fillText = function(text, x, y, ...args) {
      const bounds = this.canvas.getBoundingClientRect();
      const point = new DOMPoint(x, y - 4).matrixTransform(this.getTransform());
      window.drawnText[text] = {
        x: bounds.left + (point.x + 7) * bounds.width / this.canvas.width,
        y: bounds.top + point.y * bounds.height / this.canvas.height
      };
      return fillText.call(this, text, x, y, ...args);
    };
  ` });
  await send('Page.navigate', { url: origin });
  async function click(label) {
    const point = await until(() => value('window.drawnText?.[' + JSON.stringify(label) + ']'));
    for (const type of ['mouseMoved', 'mousePressed', 'mouseReleased']) {
      await send('Input.dispatchMouseEvent', { ...point, type, button: 'left', clickCount: 1 });
    }
  }
  async function enter(label, text) {
    await click(label);
    await until(() => value('document.activeElement.tagName === "INPUT"'));
    await send('Input.insertText', { text });
  }
  async function chooseProject(file) {
    const picker = await until(async () => (await send('DOM.querySelector', {
      nodeId: (await send('DOM.getDocument')).root.nodeId, selector: 'input[type=file]',
    })).nodeId);
    await send('DOM.setFileInputFiles', { nodeId: picker, files: [file] });
  }
  const name = 'Collection assertion fixture';
  const file = resolve(profile, 'fixture.json');
  await writeFile(file, JSON.stringify({
    format: 'protovoy-multiprotocol-project',
    version: 2,
    requests: [{ name, url: target, graphql: { query: 'query { __typename }' } }]
  }));
  await send('Page.setInterceptFileChooserDialog',{enabled:true});
  await click('Import project');
  await chooseProject(file);
  await until(()=>value('Boolean(window.drawnText["Imported 1 requests"])'));
  await value('delete window.drawnText["Expected HTTP status (collection runs)"]');
  await click(name);
  await until(()=>value('Boolean(window.drawnText["query { __typename }"])'));
  await delay(300);
  await click('Expected HTTP status (collection runs)');
  await until(()=>value('document.activeElement.tagName === "INPUT"'));
  await send('Input.insertText',{text:'404'});
  await until(()=>value('document.activeElement.value === "404"'));
  await send('Input.dispatchKeyEvent',{type:'keyDown',key:'Tab',code:'Tab',windowsVirtualKeyCode:9});
  await enter('Response body contains (collection runs)','fixture');
  await click('Save request');
  await until(()=>value('JSON.parse(localStorage.getItem("protovoy-go-workspace-v1")||"{}").project?.Requests?.[0]?.expectedHttpStatus===404')).catch(async err=>{console.log(await value('Object.keys(window.drawnText).slice(-25)'));throw err});
  await send('Page.reload',{ignoreCache:true});
  await until(()=>value('Boolean(window.drawnText["404"])'));
  const hostedFailures = [
    [401, 'sign in to the hosted workbench and try again'],
    [403, 'hosted execution was denied; open the configured workbench address'],
    [503, 'no hosted HTTP endpoint is configured; pair a Local Agent to send this request'],
  ];
  await value(`window.originalFetch = window.fetch;
    window.fetch = (url, ...args) => window.previewFailure && new URL(url, location.href).pathname === '/api/preview'
      ? Promise.resolve(new Response(null, { status: window.previewFailure }))
      : window.originalFetch(url, ...args);`);
  for (const [status, message] of hostedFailures) {
    await value('window.previewFailure = ' + status);
    await click('Send');
    await until(() => value('Boolean(window.drawnText[' + JSON.stringify(message) + '])'));
  }
  await value('window.fetch = window.originalFetch; delete window.originalFetch; delete window.previewFailure;');
  await click('Send');
  await until(() => value('Boolean(window.drawnText["HTTP 404"] && window.drawnText["upstream fixture response"])'));
  const completedTiming = 'Object.keys(window.drawnText).some(text => text.startsWith("Headers ") && text.includes(" · Preview ") && text.endsWith(" · 25 bytes"))';
  await until(() => value(completedTiming));
  await click('Response headers');
  await until(() => value('Boolean(window.drawnText["X-Fixture: first"] && window.drawnText["X-Fixture: second"] && window.drawnText["Set-Cookie: [redacted]"])'));
  assert.equal(await value('Object.keys(window.drawnText).some(text => text.includes("fixture-cookie-value"))'), false);
  if (process.env.PROTOVOY_HEADERS_SCREENSHOT) {
    const { data } = await send('Page.captureScreenshot', { format: 'png' });
    await writeFile(process.env.PROTOVOY_HEADERS_SCREENSHOT, Buffer.from(data, 'base64'));
  }
  await click('Response body');
  await click('Save preview bytes');
  const preview = await until(() => readFile(resolve(profile, 'protovoy-response-preview.bin')));
  assert.deepEqual(preview, Buffer.from('upstream fixture response'));
  if (process.env.PROTOVOY_RESPONSE_DOWNLOAD_SCREENSHOT) {
    await until(() => value('Boolean(window.drawnText["Response preview saved"])'));
    const { data } = await send('Page.captureScreenshot', { format: 'png' });
    await writeFile(process.env.PROTOVOY_RESPONSE_DOWNLOAD_SCREENSHOT, Buffer.from(data, 'base64'));
  }
  assert.equal(received.length, 1);
  assert.equal(received[0].method, 'POST');
  assert.equal(received[0].path, '/upload');
  assert.equal(JSON.parse(received[0].body).query, 'query { __typename }');
  await click('Export project');
  const path=resolve(profile,'protovoy-project.json');
  const exported=await until(async()=>JSON.parse(await readFile(path,'utf8')));
  assert.equal(exported.requests[0].expectedHttpStatus, 404);
  assert.equal(exported.requests[0].responseBodyContains, 'fixture');
  const result = await promisify(execFile)(binary('protovoy'), ['run', path]);
  assert.equal(JSON.parse(result.stdout).passed, true);
  await click('Export saved cURL');
  const downloaded = await until(() => readFile(resolve(profile, 'protovoy-curl.txt'), 'utf8'));
  const generated = await promisify(execFile)(binary('protovoy'), ['curl', path]);
  assert.equal(downloaded.trimEnd(), generated.stdout.trimEnd());
  if (process.env.PROTOVOY_SMOKE_SCREENSHOT) {
    await until(() => value('Boolean(window.drawnText["cURL command exported"])'));
    const { data } = await send('Page.captureScreenshot', { format: 'png' });
    await writeFile(process.env.PROTOVOY_SMOKE_SCREENSHOT, Buffer.from(data, 'base64'));
  }
  await click('Import cURL');
  await click('Paste a cURL command');
  await until(() => value('document.activeElement.tagName === "TEXTAREA"'));
  await send('Input.insertText', { text: downloaded.trimEnd() });
  await click('Load request draft');
  await until(() => value('Boolean(window.drawnText["cURL loaded into a new request draft"])'));
  const savedRequests = () => value('JSON.parse(localStorage.getItem("protovoy-go-workspace-v1") || "{}").project?.Requests');
  assert.equal((await savedRequests()).length, 1);
  if (process.env.PROTOVOY_CURL_IMPORT_SCREENSHOT) {
    const { data } = await send('Page.captureScreenshot', { format: 'png' });
    await writeFile(process.env.PROTOVOY_CURL_IMPORT_SCREENSHOT, Buffer.from(data, 'base64'));
  }
  await click('Save request');
  const imported = await until(async () => (await savedRequests())?.[1]);
  assert.equal(imported.url, target);
  assert.equal(imported.method, 'POST');
  assert.equal(JSON.parse(imported.body).query, 'query { __typename }');
  const har = resolve(profile, 'fixture.har');
  await writeFile(har, JSON.stringify({ log: { version: '1.2', entries: [{
    request: { method: 'POST', url: target, postData: { mimeType: 'text/plain', text: 'HAR request body' } },
  }] } }));
  await click('Import project');
  await click('Replace project');
  await chooseProject(har);
  const captured = await until(async () => {
    const requests = await savedRequests();
    return requests?.length === 1 && requests[0].body === 'HAR request body' && requests[0];
  });
  assert.equal(captured.url, target);
  assert.equal(captured.method, 'POST');
  if (process.env.PROTOVOY_HAR_SCREENSHOT) {
    const { data } = await send('Page.captureScreenshot', { format: 'png' });
    await writeFile(process.env.PROTOVOY_HAR_SCREENSHOT, Buffer.from(data, 'base64'));
  }
  const postman = resolve(profile, 'postman.json');
  await writeFile(postman, JSON.stringify({
    info: { name: 'Postman fixture', schema: 'https://schema.getpostman.com/json/collection/v2.1.0/collection.json' },
    item: [{ name: 'Fixture folder', item: [{ name: 'Postman request', request: {
      url: target, method: 'POST', body: { mode: 'raw', raw: 'Postman body' },
    } }] }],
  }));
  await click('Import project');
  await chooseProject(postman);
  const postmanRequest = await until(async () => {
    const requests = await savedRequests();
    return requests?.[0]?.body === 'Postman body' && requests[0];
  });
  assert.deepEqual(postmanRequest.collectionPath, ['Fixture folder']);
  if (process.env.PROTOVOY_POSTMAN_SCREENSHOT) {
    await until(() => value('Boolean(window.drawnText["Fixture folder"])'));
    const { data } = await send('Page.captureScreenshot', { format: 'png' });
    await writeFile(process.env.PROTOVOY_POSTMAN_SCREENSHOT, Buffer.from(data, 'base64'));
  }
  const openapi = resolve(profile, 'openapi.yaml');
  const apiName = 'Create OpenAPI fixture';
  await writeFile(openapi, `openapi: 3.1.0
info:
  title: OpenAPI fixture
  version: 1.0.0
servers:
  - url: ${new URL(target).origin}
paths:
  /upload:
    $ref: '#/components/pathItems/Upload'
components:
  pathItems:
    Upload:
      post:
        summary: ${apiName}
        requestBody:
          $ref: '#/components/requestBodies/Upload'
        responses:
          404:
            description: Fixture response
  requestBodies:
    Upload:
      content:
        application/json:
          examples:
            fixture:
              $ref: '#/components/examples/Upload'
  examples:
    Upload:
      value:
        name: openapi fixture
`);
  await click('Import project');
  await chooseProject(openapi);
  const openAPIRequest = await until(async () => {
    const requests = await savedRequests();
    return requests?.[0]?.name === apiName && requests[0];
  });
  assert.equal(openAPIRequest.url, target);
  await click(apiName);
  await until(() => value('Object.keys(window.drawnText).some(text => text.includes("openapi fixture"))'));
  const beforeSend = received.length;
  await click('Send');
  const sent = await until(() => received[beforeSend]);
  assert.equal(sent.method, 'POST');
  assert.equal(sent.path, '/upload');
  assert.deepEqual(JSON.parse(sent.body), { name: 'openapi fixture' });
  await until(() => value('Boolean(window.drawnText[' + JSON.stringify(openAPIResponse) + '])'));
  await click('Format JSON');
  await until(() => value('Object.keys(window.drawnText).some(text => text.includes(' + JSON.stringify('"fixture": "openapi ok!"') + '))'));
  if (process.env.PROTOVOY_JSON_SCREENSHOT) {
    const { data } = await send('Page.captureScreenshot', { format: 'png' });
    await writeFile(process.env.PROTOVOY_JSON_SCREENSHOT, Buffer.from(data, 'base64'));
  }
  await click('Show raw JSON');
  if (process.env.PROTOVOY_OPENAPI_SCREENSHOT) {
    const { data } = await send('Page.captureScreenshot', { format: 'png' });
    await writeFile(process.env.PROTOVOY_OPENAPI_SCREENSHOT, Buffer.from(data, 'base64'));
  }
  const beforeAdditional = received.length;
  await click('New request');
  await enter('Server-configured endpoint', additionalTarget);
  await click('Send');
  const additional = await until(() => received[beforeAdditional]);
  assert.equal(additional.path, '/additional?items=1,2');
  await until(() => value('Boolean(window.drawnText["upstream fixture response"])'));
  await click('Discard edits');
  await enter('Server-configured endpoint', new URL('/additional?items=3', target).href);
  await click('Send');
  await until(() => value('Boolean(window.drawnText["request URL is not in the hosted HTTP allowlist"])'));
  assert.equal(received.length, beforeAdditional + 1);
  if (process.env.PROTOVOY_TARGET_SCREENSHOT) {
    const { data } = await send('Page.captureScreenshot', { format: 'png' });
    await writeFile(process.env.PROTOVOY_TARGET_SCREENSHOT, Buffer.from(data, 'base64'));
  }
  await click('Discard edits');
  await click(apiName);
  await until(async () => (await fetch('http://' + agentAddress + '/health')).ok);
  await click('Local Agent');
  await enter('127.0.0.1:39217', agentAddress);
  await enter('Pairing token', agentToken);
  await click('Pair with Local Agent');
  await until(() => value('Boolean(window.drawnText["Paired · Local Agent ready"])'));
  if (process.env.PROTOVOY_SMOKE_SYSTEM_KEYCHAIN) {
    const credential = randomBytes(24).toString('hex');
    await enter('Reference name', secretReference);
    await enter('Credential value', credential);
    await delay(300);
    assert.equal(await value('Boolean(window.drawnText[' + JSON.stringify(credential) + '])'), false);
    await click('Save secret');
    await until(() => value('Boolean(window.drawnText["Secret saved"])'));
    await delay(300);
    await click('Check secret');
    await until(() => value('Boolean(window.drawnText["Secret exists"])'));
    if (process.env.PROTOVOY_SECRET_SCREENSHOT) {
      const { data } = await send('Page.captureScreenshot', { format: 'png' });
      await writeFile(process.env.PROTOVOY_SECRET_SCREENSHOT, Buffer.from(data, 'base64'));
    }
    await delay(300);
    await click('Delete secret');
    await until(() => value('Boolean(window.drawnText["Secret deleted"])'));
    await delay(300);
    await value('delete window.drawnText["Secret exists"]');
    await click('Check secret');
    await until(() => value('Boolean(window.drawnText["Secret not found"])'));
    await enter('Exact request URL', target);
    await enter('Authorization', 'Authorization');
    await enter('Credential value', boundCredential);
    await click('Bind HTTP credential');
    await until(() => value('Boolean(window.drawnText["HTTP credential bound"])'));
  }
  await delay(300);
  await click('Create Agent workspace');
  await until(() => value('Boolean(window.drawnText["Agent workspace saved"])'));
  await click('Load Agent workspace');
  await until(() => value('Boolean(window.drawnText["Agent workspace loaded"])'));
  await delay(300);
  await click('Save Agent workspace');
  await until(() => value('Boolean(window.drawnText["Agent workspace saved"])')).catch(async error => { console.log(await value("Object.keys(window.drawnText).slice(-20)")); throw error; });
  if (process.env.PROTOVOY_AGENT_WORKSPACE_SCREENSHOT) {
    const { data } = await send('Page.captureScreenshot', { format: 'png' });
    await writeFile(process.env.PROTOVOY_AGENT_WORKSPACE_SCREENSHOT, Buffer.from(data, 'base64'));
  }
  await delay(300);
  await click('Delete Agent workspace');
  await until(() => value('Boolean(window.drawnText["Agent workspace deleted"])'));
  const removedWorkspace = await fetch('http://' + agentAddress + '/v1/workspace', {
    headers: { Authorization: 'Bearer ' + agentToken },
  });
  assert.equal(removedWorkspace.status, 404);
  await click('Back to workbench');
  if (process.env.PROTOVOY_SMOKE_SYSTEM_KEYCHAIN) {
    await enter('Bound credential reference', secretReference);
    await enter('Bound request header', 'Authorization');
  }
  await value('for (const text of Object.keys(window.drawnText)) if (text.startsWith("Headers ")) delete window.drawnText[text]');
  await value('delete window.drawnText["X-Fixture: first"]; delete window.drawnText["X-Fixture: second"]; delete window.drawnText["Set-Cookie: [redacted]"]');
  await click('Send via Local Agent');
  if (process.env.PROTOVOY_SMOKE_SYSTEM_KEYCHAIN) {
    await until(() => boundCredentialReceived);
  }
  await until(() => value('Boolean(window.drawnText["HTTP 404 · Local Agent"])'));
  await until(() => value(completedTiming));
  await click('Response headers');
  await until(() => value('Boolean(window.drawnText["X-Fixture: first"] && window.drawnText["X-Fixture: second"] && window.drawnText["Set-Cookie: [redacted]"])'));
  if (process.env.PROTOVOY_AGENT_HEADERS_SCREENSHOT) {
    const { data } = await send('Page.captureScreenshot', { format: 'png' });
    await writeFile(process.env.PROTOVOY_AGENT_HEADERS_SCREENSHOT, Buffer.from(data, 'base64'));
  }
  await click('Mock rules');
  await delay(300);
  await click('Validate rules');
  await until(() => value('Boolean(window.drawnText["0 Mock rules validated"])'));
  const scenario = { id: 'browser-fixture', method: 'GET', path: '/hello/{name}', status: 201, body: 'Mock editor response' };
  await click('[]');
  await until(() => value('document.activeElement.tagName === "TEXTAREA"'));
  await send('Input.dispatchKeyEvent', { type: 'keyDown', key: 'a', code: 'KeyA', modifiers: 2, windowsVirtualKeyCode: 65 });
  await send('Input.dispatchKeyEvent', { type: 'keyUp', key: 'a', code: 'KeyA', modifiers: 2, windowsVirtualKeyCode: 65 });
  await send('Input.insertText', { text: JSON.stringify([scenario]) });
  await delay(300);
  await click('Validate rules');
  await until(() => value('Boolean(window.drawnText["1 Mock rules validated"])'));
  await click('Export Mock rules');
  const mockExport = join(profile, 'mock-rules.json');
  await until(async () => JSON.parse(await readFile(mockExport, 'utf8')));
  const mockScenarios = JSON.parse(await readFile(mockExport, 'utf8'));
  assert.equal(mockScenarios.length, 1);
  assert.equal(mockScenarios[0].id, scenario.id);
  assert.equal(mockScenarios[0].body, scenario.body);
  assert.equal(mockScenarios[0].status, scenario.status);
  await runMock(binary('mock'), ['-rules', mockExport, '-check']);
  await click('Import Mock rules');
  await chooseProject(mockExport);
  await until(() => value('Boolean(window.drawnText["Mock rules imported"])'));
  await delay(300);
  await click('Import Mock rules');
  const invalidMock = join(profile, 'invalid-mock-import.json');
  await writeFile(invalidMock, '[{"id":"broken","method":"GET","status":999}]');
  await chooseProject(invalidMock);
  await until(() => value('Boolean(window.drawnText["mock rule requires an ID, method, status from 200 to 599, and delay of at most 30000 ms"])'));
  await delay(300);
  await value('delete window.drawnText["1 Mock rules validated"]');
  await click('Validate rules');
  await until(() => value('Boolean(window.drawnText["1 Mock rules validated"])'));
  if (process.env.PROTOVOY_MOCK_SCREENSHOT) {
    const { data } = await send('Page.captureScreenshot', { format: 'png' });
    await writeFile(process.env.PROTOVOY_MOCK_SCREENSHOT, Buffer.from(data, 'base64'));
  }
  await click('Start Mock via Agent');
  const mockAddress = await until(async () => {
    const response = await fetch('http://' + agentAddress + '/v1/mock', {
      headers: { Authorization: 'Bearer ' + agentToken },
    });
    const state = await response.json();
    return state.running && state.address;
  });
  await until(() => value('Boolean(window.drawnText[' + JSON.stringify('Mock running at http://' + mockAddress) + '])'));
  const servedMock = await fetch('http://' + mockAddress + '/m1/default-project/default/hello/world');
  assert.equal(servedMock.status, scenario.status);
  assert.equal(await servedMock.text(), scenario.body);
  if (process.env.PROTOVOY_MOCK_RUNNING_SCREENSHOT) {
    const { data } = await send('Page.captureScreenshot', { format: 'png' });
    await writeFile(process.env.PROTOVOY_MOCK_RUNNING_SCREENSHOT, Buffer.from(data, 'base64'));
  }
  await delay(300);
  await click('Stop Mock via Agent');
  await until(() => value('Boolean(window.drawnText["Mock stopped"])'));
  const stoppedMock = await fetch('http://' + agentAddress + '/v1/mock', {
    headers: { Authorization: 'Bearer ' + agentToken },
  });
  assert.equal((await stoppedMock.json()).running, false);
  await assert.rejects(fetch('http://' + mockAddress + '/health'));
  console.log('Viva sent the referenced OpenAPI request, and the CLI accepted the exported assertions.');
} finally {
  if (process.env.PROTOVOY_SMOKE_SYSTEM_KEYCHAIN) {
    await fetch('http://' + agentAddress + '/v1/http-secrets/' + secretReference, {
      method: 'DELETE', headers: { Authorization: 'Bearer ' + agentToken },
    }).catch(() => {});
    await fetch('http://' + agentAddress + '/v1/secrets/' + secretReference, {
      method: 'DELETE', headers: { Authorization: 'Bearer ' + agentToken },
    }).catch(() => {});
  }
  socket?.close();
  browser.kill();
  app.kill();
  agent.kill();
  upstream.close();
}
