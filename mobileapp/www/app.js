import {
  PORT, base64url, identify, keyMessage, loginError, loginState,
  parseAddress, pool, randomHex, scanHosts,
} from './lib.js';

// The native bridge Capacitor injects into this page. Plugins are called
// through it directly, so the page needs no bundler.
const cap = window.Capacitor;
const native = (plugin, method, options = {}) => cap.nativePromise(plugin, method, options);
const platform = cap?.getPlatform?.() || 'web';

const $ = (id) => document.getElementById(id);
const SERVERS = 'agenttik.servers';
const DEVICE = 'agenttik.device';

// Requests go through the native HTTP stack: it is not bound by CORS, and
// on both platforms it stores the session cookie a sign-in sets where the
// WebView reads it.
async function http(url, options = {}) {
  return native('CapacitorHttp', 'request', {
    url,
    method: 'GET',
    connectTimeout: 5000,
    readTimeout: 5000,
    disableRedirects: true,
    ...options,
  });
}

// ---- storage ----

function servers() {
  try {
    return JSON.parse(localStorage.getItem(SERVERS)) || [];
  } catch {
    return [];
  }
}

function saveServer(entry) {
  const list = servers().filter((s) => s.origin !== entry.origin);
  list.unshift({ ...entry, used: Date.now() });
  localStorage.setItem(SERVERS, JSON.stringify(list));
}

function forgetServer(origin) {
  localStorage.setItem(SERVERS, JSON.stringify(servers().filter((s) => s.origin !== origin)));
}

// device is this phone's identity as a trusted machine: an ID made once, and
// the name the server's trusted list shows.
function device() {
  let d;
  try {
    d = JSON.parse(localStorage.getItem(DEVICE));
  } catch {}
  if (!d?.id) {
    d = { id: randomHex(16) };
    localStorage.setItem(DEVICE, JSON.stringify(d));
  }
  const os = { android: 'Android', ios: 'iOS' }[platform] || 'mobile';
  return { id: d.id, name: `agenttik on ${os}` };
}

// The Ed25519 key lives in this page's IndexedDB, made non-extractable, so
// neither a remote page nor this page's own script can read it out.
function openKeys() {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open('agenttik', 1);
    req.onupgradeneeded = () => req.result.createObjectStore('keys');
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

function idb(db, mode, fn) {
  return new Promise((resolve, reject) => {
    const req = fn(db.transaction('keys', mode).objectStore('keys'));
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

let keyPair;
async function deviceKey() {
  if (keyPair) return keyPair;
  if (!crypto.subtle) throw new Error('This WebView has no Web Crypto; update it to remember this phone.');
  const db = await openKeys();
  let pair = await idb(db, 'readonly', (s) => s.get('device'));
  if (!pair) {
    try {
      pair = await crypto.subtle.generateKey({ name: 'Ed25519' }, false, ['sign', 'verify']);
    } catch {
      throw new Error('This WebView cannot make Ed25519 keys; update it to remember this phone.');
    }
    await idb(db, 'readwrite', (s) => s.put(pair, 'device'));
  }
  const pub = base64url(await crypto.subtle.exportKey('raw', pair.publicKey));
  keyPair = { pair, pub };
  return keyPair;
}

// ---- connecting ----

let target; // { origin, info, totp } while signing in
let cancelled = false;

async function check(origin) {
  let res;
  try {
    res = await http(origin + '/api/version', { headers: { Accept: 'application/json' } });
  } catch {
    throw new Error(`Could not reach ${origin}.`);
  }
  const info = identify(res.status, res.data);
  if (!info) throw new Error('This URL did not identify itself as an agenttik instance.');
  return info;
}

// state asks the server for its front page as a browser would. A live
// session or no lock answers with the app; a lock answers with its form.
async function state(origin) {
  const res = await http(origin + '/', { headers: { Accept: 'text/html' }, responseType: 'text' });
  return loginState(res.data);
}

async function connect(origin) {
  cancelled = false;
  showBusy(`Connecting to ${origin}…`);
  try {
    const info = await check(origin);
    const known = servers().find((s) => s.origin === origin);
    const entry = { origin, name: info.name, id: info.id, trusted: !!(known?.trusted && known.id === info.id) };
    const s = await state(origin);
    if (cancelled) return;
    if (!s.locked) return open(entry);
    const signedIn = entry.trusted && (await keySignIn(origin, info.id));
    if (cancelled) return;
    if (signedIn) return open(entry);
    entry.trusted = false;
    saveServer(entry);
    target = { origin, info, totp: s.totp };
    showLogin();
  } catch (err) {
    if (!cancelled) showHome(err.message);
  }
}

// keySignIn answers the server's challenge with this phone's key. Any
// refusal falls back to the password form.
async function keySignIn(origin, serverID) {
  if (!serverID) return false;
  try {
    const { pair, pub } = await deviceKey();
    const ch = await http(origin + '/__auth/challenge', { headers: { Accept: 'application/json' }, responseType: 'json' });
    const nonce = typeof ch.data === 'string' ? JSON.parse(ch.data).nonce : ch.data?.nonce;
    if (ch.status !== 200 || !nonce) return false;
    const signature = base64url(await crypto.subtle.sign('Ed25519', pair.privateKey, keyMessage(serverID, nonce)));
    const me = device();
    const res = await http(origin + '/__auth/key', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      data: JSON.stringify({ key: pub, nonce, signature, id: me.id, name: me.name }),
    });
    return res.status === 204;
  } catch {
    return false;
  }
}

async function passwordSignIn(password, code) {
  const { origin, info } = target;
  const me = device();
  const form = new URLSearchParams({ password, code, next: '/', id: me.id, name: me.name });
  let keyError = '';
  if (info.id) {
    try {
      form.set('key', (await deviceKey()).pub);
    } catch (err) {
      keyError = err.message;
    }
  }
  const res = await http(origin + '/__auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-www-form-urlencoded', Accept: 'text/html' },
    data: form.toString(),
    responseType: 'text',
  });
  if (res.status !== 303) throw new Error(loginError(res.data));
  if (keyError) alert(`Signed in, but this phone will ask again next time: ${keyError}`);
  return open({ origin, name: info.name, id: info.id, trusted: !!info.id && !keyError });
}

// open hands the WebView over to the remote. The native side keeps that one
// origin inside the app; every other link still opens in the system browser.
async function open(entry) {
  saveServer(entry);
  // The session cookie came back on a native request; reading it back makes
  // sure the WebView's store holds it before navigating.
  await native('CapacitorCookies', 'getCookies', { url: entry.origin }).catch(() => {});
  await native('AgenttikNative', 'allow', { origin: entry.origin });
  location.href = entry.origin + '/';
}

// ---- finding servers ----

async function scan() {
  const list = $('found');
  list.replaceChildren();
  $('scan').disabled = true;
  $('scan-status').textContent = 'Looking on this network…';
  let found = 0;
  try {
    const { addresses } = await native('AgenttikNative', 'addresses');
    const hosts = scanHosts(addresses);
    if (!hosts.length) throw new Error('This phone is not on a private IPv4 network.');
    await pool(hosts, 32, async (host) => {
      const origin = `http://${host}:${PORT}`;
      try {
        const res = await http(origin + '/api/version', { connectTimeout: 500, readTimeout: 500, headers: { Accept: 'application/json' } });
        const info = identify(res.status, res.data);
        if (!info) return;
        found++;
        list.append(serverButton({ origin, name: info.name, version: info.version }));
      } catch {}
    });
    $('scan-status').textContent = found ? '' : `No agenttik found on port ${PORT}.`;
  } catch (err) {
    $('scan-status').textContent = err.message;
  } finally {
    $('scan').disabled = false;
  }
}

// ---- views ----

function serverButton(s, { removable = false } = {}) {
  const li = document.createElement('li');
  const b = document.createElement('button');
  b.className = 'server';
  b.innerHTML = '<strong></strong><span></span>';
  b.querySelector('strong').textContent = s.name || s.origin;
  b.querySelector('span').textContent = [s.origin, s.version, s.trusted ? 'remembered' : ''].filter(Boolean).join(' · ');
  b.onclick = () => connect(s.origin);
  li.append(b);
  if (removable) {
    const x = document.createElement('button');
    x.className = 'forget';
    x.title = 'Forget';
    x.textContent = '×';
    x.onclick = () => {
      forgetServer(s.origin);
      showHome();
    };
    li.append(x);
  }
  return li;
}

function show(view) {
  for (const v of ['home', 'busy', 'login']) $(v).hidden = v !== view;
}

function showHome(error = '') {
  $('error').textContent = error;
  const saved = servers();
  $('saved-section').hidden = !saved.length;
  $('saved').replaceChildren(...saved.map((s) => serverButton(s, { removable: true })));
  show('home');
}

function showBusy(text) {
  $('busy-text').textContent = text;
  show('busy');
}

function showLogin(error = '') {
  $('login-title').textContent = target.info.name || target.origin;
  $('login-origin').textContent = target.origin;
  $('code-row').hidden = !target.totp;
  $('code').required = target.totp;
  $('login-error').textContent = error;
  $('password').value = '';
  $('code').value = '';
  show('login');
  $('password').focus();
}

$('connect-form').onsubmit = (e) => {
  e.preventDefault();
  try {
    connect(parseAddress($('address').value));
  } catch (err) {
    showHome(err.message);
  }
};

$('login-form').onsubmit = async (e) => {
  e.preventDefault();
  const button = $('login-form').querySelector('button[type=submit]');
  button.disabled = true;
  try {
    await passwordSignIn($('password').value, $('code').value.trim());
  } catch (err) {
    showLogin(err.message);
  } finally {
    button.disabled = false;
  }
};

$('scan').onclick = scan;
$('cancel').onclick = () => {
  cancelled = true;
  showHome();
};
$('back').onclick = () => showHome();

// A cold start goes straight back to the last server. Coming back to this
// page from the remote, within the same run, does not.
showHome();
const last = servers()[0];
if (last && !sessionStorage.getItem('started')) connect(last.origin);
sessionStorage.setItem('started', '1');
