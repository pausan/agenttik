import {
  PORT, base64url, candidates, identify, keyMessage, knownOrigins, locate,
  loginError, loginState, parseAddress, pool, randomHex, remember, scanHosts,
} from './lib.js';

// The native bridge Capacitor injects into this page. Plugins are called
// through it directly, so the page needs no bundler.
const cap = window.Capacitor;
const native = (plugin, method, options = {}) => cap.nativePromise(plugin, method, options);
const platform = cap?.getPlatform?.() || 'web';

const $ = (id) => document.getElementById(id);
const SERVERS = 'agenttik.servers';
const DEVICE = 'agenttik.device';
// SIGNED_OUT holds the origin this phone last logged out of. While it is set,
// a cold start stays on this page instead of reconnecting.
const SIGNED_OUT = 'agenttik.signedOut';

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

// saveServer puts entry first, merged with what is saved for the same
// server, plus the origins it reported.
function saveServer(entry, learned = []) {
  localStorage.setItem(SERVERS, JSON.stringify(remember(servers(), { ...entry, used: Date.now() }, learned)));
}

function savedServer(origin) {
  return servers().find((s) => knownOrigins(s).includes(origin));
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
let attempt = 0;
let retry;

function retryConnection(origin) {
  showBusy(`Connection lost. Retrying ${origin} in 3 seconds…`);
  retry = setTimeout(() => connect(origin, true), 3000);
}

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
  if (res.status !== 200) throw new Error(`Could not load ${origin} (HTTP ${res.status}).`);
  return loginState(res.data);
}

// reach finds where a saved server answers now: its local network origins
// first, then the most recently reached. Others answer like check.
async function reach(origin, known) {
  if (!known?.id || knownOrigins(known).length < 2) return { origin, info: await check(origin) };
  return locate(candidates({ ...known, origin }), async (o) => {
    const info = await check(o);
    if (info.id !== known.id) throw new Error(`Another agenttik answers at ${o}.`);
    return info;
  });
}

async function connect(origin, reconnect = false) {
  clearTimeout(retry);
  const current = ++attempt;
  showBusy(`${reconnect ? 'Reconnecting' : 'Connecting'} to ${origin}…`);
  try {
    const known = savedServer(origin);
    let info;
    ({ origin, info } = await reach(origin, known));
    if (current !== attempt) return;
    const entry = { origin, name: info.name, id: info.id, trusted: !!(known?.trusted && known.id === info.id) };
    const s = await state(origin);
    if (current !== attempt) return;
    if (!s.locked) return await open(entry, current);
    const signedIn = entry.trusted && (await keySignIn(origin, info.id));
    if (current !== attempt) return;
    if (signedIn) return await open(entry, current);
    entry.trusted = false;
    saveServer(entry);
    target = { origin, info, totp: s.totp };
    showLogin();
  } catch (err) {
    if (current !== attempt) return;
    if (reconnect) {
      retryConnection(origin);
    } else {
      showHome(err.message);
    }
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
  const current = attempt;
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
  return open({ origin, name: info.name, id: info.id, trusted: !!info.id && !keyError }, current);
}

// addresses asks a signed-in server where else it can be reached. Older
// servers do not say.
async function addresses(origin) {
  try {
    const res = await http(origin + '/api/server/addresses', { connectTimeout: 2000, readTimeout: 2000, headers: { Accept: 'application/json' } });
    const data = typeof res.data === 'string' ? JSON.parse(res.data) : res.data;
    return res.status === 200 && Array.isArray(data?.addresses) ? data.addresses.map(String) : [];
  } catch {
    return [];
  }
}

// open hands the WebView over to the remote. The native side keeps that one
// origin inside the app; every other link still opens in the system browser.
async function open(entry, current) {
  // The session cookie came back on a native request; reading it back makes
  // sure the WebView's store holds it before navigating.
  await native('CapacitorCookies', 'getCookies', { url: entry.origin }).catch(() => {});
  if (current !== attempt) return;
  const learned = entry.id ? await addresses(entry.origin) : [];
  if (current !== attempt) return;
  await native('AgenttikNative', 'allow', { origin: entry.origin });
  if (current !== attempt) return;
  saveServer(entry, learned);
  localStorage.removeItem(SIGNED_OUT);
  location.href = entry.origin + '/';
}

// ---- finding servers ----

async function scan() {
  const list = $('found');
  list.replaceChildren();
  $('scan').disabled = true;
  $('scan-status').textContent = 'Looking on this network…';
  let found = 0;
  const saved = new Map(servers().flatMap((s) => knownOrigins(s).map((o) => [o, s])));
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
        list.append(serverButton({ ...saved.get(origin), origin, name: info.name, version: info.version }, { known: saved.has(origin) }));
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

// serverButton shows one server. known marks a scan result that is also
// saved; the server this phone logged out of says so.
function serverButton(s, { removable = false, known = false } = {}) {
  const li = document.createElement('li');
  const b = document.createElement('button');
  b.className = known ? 'server known' : 'server';
  b.innerHTML = '<strong></strong><span></span>';
  b.querySelector('strong').textContent = s.name || s.origin;
  const signedOut = localStorage.getItem(SIGNED_OUT) === s.origin;
  const more = knownOrigins(s).length - 1;
  b.title = knownOrigins(s).join('\n');
  b.querySelector('span').textContent = [
    s.origin, more > 0 ? `+${more} more` : '', s.version, signedOut ? 'signed out' : '',
    s.trusted ? 'remembered' : known ? 'saved' : '',
  ].filter(Boolean).join(' · ');
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
  clearTimeout(retry);
  attempt++;
  showHome();
};
$('back').onclick = () => {
  attempt++;
  showHome();
};

// Log out in the remote's Settings comes back here with #signed-out.
if (location.hash === '#signed-out') {
  localStorage.setItem(SIGNED_OUT, servers()[0]?.origin || '');
  history.replaceState(null, '', location.pathname);
}

// A cold start goes straight back to the last server, unless this phone
// logged out. Coming back to this page from the remote, within the same run,
// does not.
// A failed remote document has lost all of its JavaScript, including SSE's
// reconnect. Android brings it here so retries work even while LAN is down.
let lostOrigin;
if (location.hash.startsWith('#connection-lost=')) {
  try {
    lostOrigin = parseAddress(decodeURIComponent(location.hash.slice('#connection-lost='.length)));
  } catch {}
  history.replaceState(null, '', location.pathname);
}
showHome();
const last = servers()[0];
if (lostOrigin) retryConnection(lostOrigin);
else if (last && !sessionStorage.getItem('started') && localStorage.getItem(SIGNED_OUT) === null) connect(last.origin);
sessionStorage.setItem('started', '1');
