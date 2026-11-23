import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {mkdtemp,readFile,writeFile} from 'node:fs/promises';
import {createServer} from 'node:http';
import {resolve,join} from 'node:path';
import {tmpdir} from 'node:os';
import {setTimeout as delay} from 'node:timers/promises';
import assert from 'node:assert/strict';
const fixture=createServer((req,res)=>{res.end('scheduled fixture');});
fixture.listen(0,'127.0.0.1');await once(fixture,'listening');
const target='http://127.0.0.1:'+fixture.address().port+'/';
const bundle=resolve(process.argv[2]||'build/native');
const port=process.env.PROTOVOY_SCHEDULE_SMOKE_PORT||'39317';
const origin='http://127.0.0.1:'+port;
const binary=join(bundle,'server'+(process.platform==='win32'?'.exe':''));
const chrome=process.env.CHROME_BIN||'google-chrome';
const data=await mkdtemp(join(tmpdir(),'protovoy-viva-schedules-'));
const app=spawn(binary,['-port',port,'-web',join(bundle,'web'),'-target',target,'-schedule-db',resolve(data,'schedules.db')],{stdio:'inherit',windowsHide:true});
const profile = await mkdtemp(join(tmpdir(),'protovoy-viva-schedules-chrome-'));
const browser = spawn(chrome, [
  '--headless', '--disable-background-networking', '--no-first-run', '--disable-extensions',
  '--remote-debugging-port=0', '--force-device-scale-factor=1', '--window-size=1200,1800',
  '--user-data-dir=' + profile, 'about:blank',
], { stdio: 'ignore', windowsHide: true });

async function until(fn) {
  for (let attempt = 0; attempt < 300; attempt++) {
    try {
      const value = await fn();
      if (value) return value;
    } catch {}
    await delay(100);
  }
  throw new Error('Timed out waiting for ' + fn.toString().slice(0, 160));
}

for(const child of [app,browser]) child.on('error',error=>console.error(error.message));
let socket; let diagnostics;
try {
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
  diagnostics=async()=>{await writeFile(join(data,'schedules-failure.png'),Buffer.from((await send('Page.captureScreenshot',{format:'png'})).data,'base64'));return value('Object.keys(window.drawnText)');};
  await send('Page.enable');
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
      await send('Input.dispatchMouseEvent', { ...point, type, button: 'left', clickCount: 1 }); await delay(80);
    }
  }
  async function enter(label,text) {
    await click(label);
    await until(() => value('["INPUT","TEXTAREA"].includes(document.activeElement.tagName)'));
    await send('Input.insertText',{text});
  }

  let response=await fetch(origin+'/v1/jobs',{method:'POST',headers:{Origin:origin,'X-ProtoVoy-Scheduler':'1','Content-Type':'application/json'},body:JSON.stringify({name:'Browser schedule',intervalSeconds:10,enabled:false,request:{name:'Fixture',method:'GET',url:target}})});
  assert.equal(response.status,201);
  const job=await response.json();
  await click('Schedules');
  await click('Connect to schedules');
  try { await until(()=>value('Boolean(window.drawnText["Schedules updated"])')); }
  catch(error) { console.log(await value('Object.keys(window.drawnText)')); await writeFile(join(data,'schedules-connection-failure.png'),Buffer.from((await send('Page.captureScreenshot',{format:'png'})).data,'base64')); throw error; }
  await click('Enable Browser schedule');
  await until(async()=>{
    const r=await fetch(origin+'/v1/jobs',{headers:{Origin:origin,'X-ProtoVoy-Scheduler':'1'}});
    return (await r.json()).find(x=>x.id===job.id)?.enabled;
  });
  await until(async()=>{
    const r=await fetch(origin+'/v1/executions',{headers:{Origin:origin,'X-ProtoVoy-Scheduler':'1'}});
    return (await r.json()).some(x=>x.jobId===job.id&&x.success);
  });
  await click('Refresh schedules');
  await until(()=>value('Object.keys(window.drawnText).some(t=>t.includes("Browser schedule · Passed"))'));
  await writeFile(process.env.PROTOVOY_SCHEDULE_SCREENSHOT||join(data,'schedules.png'),Buffer.from((await send('Page.captureScreenshot',{format:'png'})).data,'base64'));
  await value('delete window.drawnText["Schedules updated"]; true');
  await click('Disable Browser schedule');
  await until(async()=>{
    const r=await fetch(origin+'/v1/jobs',{headers:{Origin:origin,'X-ProtoVoy-Scheduler':'1'}});
    return (await r.json()).find(x=>x.id===job.id)?.enabled===false;
  });
  await until(()=>value('Boolean(window.drawnText["Schedules updated"])'));
  await click('Delete Browser schedule');
  await until(async()=>{
    const r=await fetch(origin+'/v1/jobs',{headers:{Origin:origin,'X-ProtoVoy-Scheduler':'1'}});
    return (await r.json()).length===0;
  });
  console.log('Viva schedules browser smoke passed: connect, enable, actual execution, refresh, disable and delete.');
} catch(error) { console.log(await diagnostics?.()); throw error; } finally {
  socket?.close();browser.kill();app.kill();fixture.close();
}