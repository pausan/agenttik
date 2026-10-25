import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import http from 'node:http';
import test from 'node:test';
import { setTimeout } from 'node:timers/promises';
import { promisify } from 'node:util';

// Uses a real Android WebView and an isolated HTTP server. Install a debug
// APK in an emulator first; this deliberately refuses physical devices.
const exec = promisify(execFile);
const serial = process.env.ANDROID_SERIAL || '';
const adb = async (...args) => (await exec('adb', ['-s', serial, ...args])).stdout.trim();
const app = 'com.pausan.agenttik';

async function until(fn) {
  const deadline = Date.now() + 20000;
  while (Date.now() < deadline) {
    const result = await fn();
    if (result) return result;
    await setTimeout(100);
  }
  throw new Error('Timed out waiting for Android WebView');
}

test('Android recovers after losing a connected server', { timeout: 90000 }, async (t) => {
  assert.match(serial, /^emulator-\d+$/, 'Set ANDROID_SERIAL to a running emulator with the debug APK installed');
  let offline = false;
  let requests = 0;
  const server = http.createServer((req, res) => {
    requests++;
    if (offline || req.url === '/broken.png') return req.socket.destroy();
    if (req.url === '/api/version') {
      res.setHeader('Content-Type', 'application/json');
      return res.end(JSON.stringify({ application: 'agenttik', version: 'test', name: 'Reconnect fixture' }));
    }
    res.setHeader('Content-Type', 'text/html');
    res.end('<!doctype html><title>Connected fixture</title><body>Connected fixture</body>');
  });
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  t.after(() => { server.closeAllConnections(); server.close(); });
  const port = server.address().port;
  const origin = `http://127.0.0.1:${port}`;
  await adb('reverse', `tcp:${port}`, `tcp:${port}`);
  t.after(() => adb('reverse', '--remove', `tcp:${port}`));
  await adb('shell', 'am', 'force-stop', app);
  await adb('shell', 'am', 'start', '-n', `${app}/.MainActivity`);
  const pid = await until(() => adb('shell', 'pidof', app).catch(() => ''));
  await until(async () => (await adb('shell', 'cat', '/proc/net/unix')).includes(`@webview_devtools_remote_${pid}`));
  const debugPort = await adb('forward', 'tcp:0', `localabstract:webview_devtools_remote_${pid}`);
  t.after(() => adb('forward', '--remove', `tcp:${debugPort}`));
  const target = await until(async () => (await (await fetch(`http://127.0.0.1:${debugPort}/json`)).json())[0]);
  const socket = new WebSocket(target.webSocketDebuggerUrl);
  t.after(() => socket.close());
  await new Promise((resolve, reject) => { socket.onopen = resolve; socket.onerror = reject; });
  let next = 0;
  const pending = new Map();
  socket.onmessage = ({ data }) => {
    const message = JSON.parse(data);
    if (!message.id) return;
    pending.get(message.id)(message);
    pending.delete(message.id);
  };
  async function evaluate(expression) {
    const id = ++next;
    const response = await new Promise((resolve) => {
      pending.set(id, resolve);
      socket.send(JSON.stringify({ id, method: 'Runtime.evaluate', params: { expression, returnByValue: true, awaitPromise: true } }));
    });
    assert.ok(!response.error && !response.result.exceptionDetails, JSON.stringify(response));
    return response.result.result.value;
  }
  const waitFor = (expression) => until(() => evaluate(expression));
  await waitFor('!!document.getElementById("address")');
  await evaluate('localStorage.clear(); sessionStorage.setItem("started", "1")');
  const connect = async () => {
    await evaluate(`document.getElementById('address').value=${JSON.stringify(origin)}; document.getElementById('connect-form').requestSubmit()`);
    await waitFor('document.title === "Connected fixture"');
  };

  await t.test('opens plain HTTP without giving the remote a Capacitor bridge', async () => {
    await connect();
    assert.equal(await evaluate('location.origin'), origin);
    assert.equal(await evaluate('typeof window.Capacitor'), 'undefined');
  });

  await t.test('a failed subresource keeps the connected UI', async () => {
    await evaluate("window.imageFailed=false; const img=new Image(); img.onerror=()=>window.imageFailed=true; img.src='/broken.png'; document.body.append(img)");
    await waitFor('window.imageFailed');
    assert.equal(await evaluate('document.title'), 'Connected fixture');
  });

  async function loseConnection() {
    offline = true;
    await evaluate('location.reload()');
    await waitFor('location.hostname === "localhost" && !!document.getElementById("busy") && !document.getElementById("busy").hidden');
    assert.match(await evaluate('document.getElementById("busy-text").textContent'), /Connection lost|Reconnecting/);
  }

  await t.test('a failed reload returns to the bundled page and reconnects automatically', async () => {
    await loseConnection();
    const failed = requests;
    await until(() => requests > failed);
    offline = false;
    await waitFor('document.title === "Connected fixture"');
    assert.equal(await evaluate('location.origin'), origin);
  });

  await t.test('Cancel stops retries while the server stays unavailable', async () => {
    await loseConnection();
    await evaluate('document.getElementById("cancel").click()');
    await waitFor('!document.getElementById("home").hidden');
    const stopped = requests;
    await setTimeout(4000);
    assert.equal(requests, stopped);
    assert.equal(await evaluate('location.hostname'), 'localhost');
  });

  await t.test('manual reconnect and logout still work', async () => {
    offline = false;
    await connect();
    await evaluate('window.agenttikApp.postMessage("disconnect")');
    await waitFor('location.hostname === "localhost" && !!document.getElementById("home") && !document.getElementById("home").hidden');
    assert.equal(await evaluate('localStorage.getItem("agenttik.signedOut")'), origin);
  });
});
