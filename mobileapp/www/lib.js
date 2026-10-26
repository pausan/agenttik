// Pure helpers for the connect page. No DOM and no native calls, so Node's
// test runner covers them (test/lib.test.js).

export const PORT = 7717;

// parseAddress turns what was typed into an origin, with the same rules as
// the desktop's remote.Parse: a bare host:port means HTTP, and paths,
// queries, fragments and credentials are refused.
export function parseAddress(input) {
  let s = String(input).trim();
  if (!s.includes('://')) s = 'http://' + s;
  let u;
  try {
    u = new URL(s);
  } catch {
    throw new Error('Enter a host:port or an HTTP(S) URL.');
  }
  const pathPart = s.slice(s.indexOf('://') + 3).replace(/^[^/?#]*/, '');
  if ((u.protocol !== 'http:' && u.protocol !== 'https:') || !u.hostname || u.username || u.password ||
      (pathPart !== '' && pathPart !== '/')) {
    throw new Error('Enter a host:port or an HTTP(S) URL without credentials, a path, query, or fragment.');
  }
  return u.origin;
}

// identify accepts only the agenttik signature at /api/version, as the
// desktop does. data is the parsed body or its text.
export function identify(status, data) {
  let info = data;
  if (typeof info === 'string') {
    if (info.length > 4096) return null;
    try {
      info = JSON.parse(info);
    } catch {
      return null;
    }
  }
  if (status !== 200 || !info || info.application !== 'agenttik' || typeof info.version !== 'string' || !info.version.trim()) {
    return null;
  }
  return { version: info.version, name: info.name || '', id: info.id || '' };
}

// loginState reads the page a server answers a navigation with: its own
// login form when the lock is on, the app otherwise.
export function loginState(html) {
  html = String(html || '');
  if (!html.includes('action="/__auth/login"')) return { locked: false, totp: false };
  return { locked: true, totp: html.includes('name="code"') };
}

// loginError pulls the reason out of a refused login form.
export function loginError(html) {
  const m = /<p class="bad">([^<]*)<\/p>/.exec(String(html || ''));
  return m ? decodeEntities(m[1]) : 'Sign-in failed.';
}

function decodeEntities(s) {
  return s.replace(/&(#\d+|#x[0-9a-f]+|amp|lt|gt|quot|apos|rsaquo);/gi, (_, e) => {
    const named = { amp: '&', lt: '<', gt: '>', quot: '"', apos: "'", rsaquo: '›' }[e.toLowerCase()];
    if (named) return named;
    return String.fromCodePoint(e[1].toLowerCase() === 'x' ? parseInt(e.slice(2), 16) : parseInt(e.slice(1), 10));
  });
}

// scanHosts lists the addresses to probe: every other host on the /24 of
// each private IPv4 address the phone has, most local first, each once.
// The desktop goes on to /16 and wider; a phone stops at the /24 to keep the
// scan to seconds and the battery out of it.
export function scanHosts(addresses) {
  const own = new Set(addresses);
  const seen = new Set();
  const hosts = [];
  for (const addr of addresses) {
    if (!isPrivateIPv4(addr)) continue;
    const prefix = addr.split('.').slice(0, 3).join('.');
    if (seen.has(prefix)) continue;
    seen.add(prefix);
    for (let i = 1; i < 255; i++) {
      const host = `${prefix}.${i}`;
      if (!own.has(host)) hosts.push(host);
    }
  }
  return hosts;
}

export function isPrivateIPv4(addr) {
  const p = String(addr).split('.').map(Number);
  if (p.length !== 4 || p.some((n) => !Number.isInteger(n) || n < 0 || n > 255)) return false;
  return p[0] === 10 || (p[0] === 172 && p[1] >= 16 && p[1] <= 31) || (p[0] === 192 && p[1] === 168);
}

// keyMessage is what a trusted machine signs to sign in; it must match
// netauth.KeyMessage byte for byte.
export function keyMessage(serverID, nonce) {
  return new TextEncoder().encode(`agenttik-key-login-v1\n${serverID}\n${nonce}`);
}

export function base64url(bytes) {
  let s = '';
  for (const b of new Uint8Array(bytes)) s += String.fromCharCode(b);
  return btoa(s).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

export function randomHex(n) {
  const b = crypto.getRandomValues(new Uint8Array(n));
  return Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('');
}

// pool runs worker over items with at most n in flight.
export async function pool(items, n, worker) {
  let next = 0;
  const run = async () => {
    while (next < items.length) await worker(items[next++]);
  };
  await Promise.all(Array.from({ length: Math.min(n, items.length) }, run));
}
