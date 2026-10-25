import assert from 'node:assert/strict';
import test from 'node:test';
import { setImmediate } from 'node:timers/promises';

const origin = 'http://192.168.4.36:7717';
let fixture = 0;

// Run the actual page with a small DOM and native bridge. Timers are manual
// so a lost network never makes these tests wait or send real requests.
async function page(t, { lost = false, request, nativeCall } = {}) {
  const elements = new Map();
  let created = 0;
  function element(id) {
    if (!elements.has(id)) elements.set(id, {
      hidden: true, value: '', textContent: '',
      replaceChildren() {}, append() {}, focus() {},
      querySelector: (selector) => element(id + selector),
    });
    return elements.get(id);
  }
  const data = new Map();
  const storage = {
    getItem: (key) => data.get(key) ?? null,
    setItem: (key, value) => data.set(key, String(value)),
    removeItem: (key) => data.delete(key),
  };
  const timers = new Map();
  const calls = [];
  const globals = {
    document: { getElementById: element, createElement: () => element(`created-${++created}`) },
    location: { hash: lost ? '#connection-lost=' + encodeURIComponent(origin) : '', pathname: '/', href: 'https://localhost/' },
    history: { replaceState() { globals.location.hash = ''; } },
    localStorage: storage,
    sessionStorage: { ...storage, getItem: () => '1' },
    setTimeout: (fn, delay) => { timers.set(fn, delay); return fn; },
    clearTimeout: (id) => timers.delete(id),
    window: { Capacitor: {
      getPlatform: () => 'android',
      async nativePromise(plugin, method, options) {
        calls.push({ plugin, method, options });
        if (plugin !== 'CapacitorHttp') return nativeCall ? nativeCall(plugin, method, options) : {};
        if (request) return request(options);
        return options.url.endsWith('/api/version')
          ? { status: 200, data: { application: 'agenttik', version: 'test' } }
          : { status: 200, data: '<div id="app"></div>' };
      },
    } },
  };
  const previous = Object.fromEntries(Object.keys(globals).map((key) => [key, Object.getOwnPropertyDescriptor(globalThis, key)]));
  for (const [key, value] of Object.entries(globals)) Object.defineProperty(globalThis, key, { configurable: true, writable: true, value });
  t.after(() => {
    for (const [key, descriptor] of Object.entries(previous)) {
      if (descriptor) Object.defineProperty(globalThis, key, descriptor);
      else delete globalThis[key];
    }
  });
  await import(`../www/app.js?fixture=${++fixture}`);
  const flush = async () => { for (let i = 0; i < 4; i++) await setImmediate(); };
  return {
    ...globals, element, calls, timers, flush,
    async tick() {
      const [fn, delay] = timers.entries().next().value;
      assert.equal(delay, 3000);
      timers.delete(fn);
      fn();
      await flush();
    },
    submit() {
      element('address').value = origin;
      element('connect-form').onsubmit({ preventDefault() {} });
    },
  };
}

test('a lost document retries until reachable, then hands over to the remote', async (t) => {
  let reachable = false;
  const p = await page(t, { lost: true, request: async ({ url }) => {
    if (!reachable) throw new Error('offline');
    return url.endsWith('/api/version')
      ? { status: 200, data: { application: 'agenttik', version: 'test' } }
      : { status: 200, data: '<div id="app"></div>' };
  } });
  assert.equal(p.location.hash, '');
  assert.match(p.element('busy-text').textContent, /Connection lost/);
  await p.tick();
  assert.equal(p.location.href, 'https://localhost/');
  assert.equal(p.timers.size, 1);
  reachable = true;
  await p.tick();
  assert.equal(p.location.href, origin + '/');
  assert.equal(p.timers.size, 0);
  assert.ok(p.calls.some((c) => c.plugin === 'AgenttikNative' && c.options.origin === origin));
});

test('Cancel stops scheduled recovery', async (t) => {
  const p = await page(t, { lost: true });
  p.element('cancel').onclick();
  assert.equal(p.timers.size, 0);
  assert.equal(p.element('home').hidden, false);
  assert.equal(p.calls.length, 0);
});

test('a cancelled in-flight attempt cannot override a newer connection', async (t) => {
  const pending = [];
  const p = await page(t, { request: (options) => new Promise((resolve) => pending.push({ options, resolve })) });
  p.submit();
  p.element('cancel').onclick();
  p.submit();
  pending[0].resolve({ status: 200, data: { application: 'agenttik', version: 'test' } });
  await p.flush();
  assert.equal(pending.length, 2);
  assert.equal(p.location.href, 'https://localhost/');
  assert.ok(!p.calls.some((c) => c.plugin === 'AgenttikNative'));
  pending[1].resolve({ status: 200, data: { application: 'agenttik', version: 'test' } });
  await p.flush();
  pending[2].resolve({ status: 200, data: '<div id="app"></div>' });
  await p.flush();
  assert.equal(p.location.href, origin + '/');
});

test('an unavailable front page is retried without navigating', async (t) => {
  const p = await page(t, { lost: true, request: async ({ url }) => url.endsWith('/api/version')
    ? { status: 200, data: { application: 'agenttik', version: 'test' } }
    : { status: 503, data: 'unavailable' } });
  await p.tick();
  assert.equal(p.location.href, 'https://localhost/');
  assert.equal(p.timers.size, 1);
});

test('Cancel during cookie transfer prevents the native handoff', async (t) => {
  let complete;
  const p = await page(t, { nativeCall: (plugin) => plugin === 'CapacitorCookies'
    ? new Promise((resolve) => { complete = resolve; }) : {} });
  p.submit();
  await p.flush();
  p.element('cancel').onclick();
  complete({});
  await p.flush();
  assert.equal(p.location.href, 'https://localhost/');
  assert.ok(!p.calls.some((c) => c.plugin === 'AgenttikNative'));
});

test('a failed native handoff is caught and shown on the address page', async (t) => {
  const p = await page(t, { nativeCall: (plugin) => {
    if (plugin === 'AgenttikNative') throw new Error('Cannot open server');
    return {};
  } });
  p.submit();
  await p.flush();
  assert.equal(p.location.href, 'https://localhost/');
  assert.equal(p.element('home').hidden, false);
  assert.equal(p.element('error').textContent, 'Cannot open server');
});

test('recovery asks for sign-in when the server session expired', async (t) => {
  const p = await page(t, { lost: true, request: async ({ url }) => url.endsWith('/api/version')
    ? { status: 200, data: { application: 'agenttik', version: 'test' } }
    : { status: 200, data: '<form action="/__auth/login"><input name="code"></form>' } });
  await p.tick();
  assert.equal(p.element('login').hidden, false);
  assert.equal(p.timers.size, 0);
  assert.equal(p.location.href, 'https://localhost/');
});
