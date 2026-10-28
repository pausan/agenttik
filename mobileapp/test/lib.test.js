import assert from 'node:assert/strict';
import test from 'node:test';
import {
  MAX_ORIGINS, base64url, candidates, identify, isLocalOrigin, isPrivateIPv4,
  keyMessage, locate, loginError, loginState, parseAddress, pool, remember,
  scanHosts,
} from '../www/lib.js';

test('parseAddress follows the desktop rules', () => {
  assert.equal(parseAddress(' 192.168.1.20:7717 '), 'http://192.168.1.20:7717');
  assert.equal(parseAddress('https://a.trycloudflare.com/'), 'https://a.trycloudflare.com');
  assert.equal(parseAddress('[fd00::1]:7717'), 'http://[fd00::1]:7717');
  for (const bad of ['', 'ftp://x', 'http://u:p@x', 'x:7717/path', 'x?q=1', 'x#f', 'x:99999']) {
    assert.throws(() => parseAddress(bad), Error, bad);
  }
});

test('identify accepts only the agenttik signature', () => {
  const ok = { application: 'agenttik', version: '1.2.3', name: 'box', id: 'abc' };
  assert.deepEqual(identify(200, ok), { version: '1.2.3', name: 'box', id: 'abc' });
  assert.deepEqual(identify(200, JSON.stringify(ok)), { version: '1.2.3', name: 'box', id: 'abc' });
  assert.equal(identify(404, ok), null);
  assert.equal(identify(200, { application: 'other', version: '1' }), null);
  assert.equal(identify(200, { application: 'agenttik', version: ' ' }), null);
  assert.equal(identify(200, '<html>'), null);
  assert.equal(identify(200, 'x'.repeat(5000)), null);
});

test('loginState reads the lock and 2FA from the front page', () => {
  assert.deepEqual(loginState('<div id="app"></div>'), { locked: false, totp: false });
  assert.deepEqual(loginState('<form method="post" action="/__auth/login"><input name="password">'), { locked: true, totp: false });
  assert.deepEqual(loginState('<form method="post" action="/__auth/login"><input id="code" name="code">'), { locked: true, totp: true });
});

test('loginError pulls the reason out of the form', () => {
  assert.equal(loginError('<p class="bad">Wrong password or code.</p>'), 'Wrong password or code.');
  assert.equal(loginError('<p class="bad">Settings &rsaquo; Server &amp; more</p>'), 'Settings › Server & more');
  assert.equal(loginError(''), 'Sign-in failed.');
});

test('scanHosts covers each private /24 once and skips the phone', () => {
  const hosts = scanHosts(['192.168.1.20', '192.168.1.30', '100.64.0.1', '10.0.0.5']);
  assert.equal(hosts.length, 252 + 253);
  assert.equal(hosts[0], '192.168.1.1');
  assert.ok(!hosts.includes('192.168.1.20') && !hosts.includes('192.168.1.30') && !hosts.includes('10.0.0.5'));
  assert.ok(hosts.includes('10.0.0.254'));
  assert.ok(!hosts.some((h) => h.startsWith('100.64.')));
  assert.deepEqual(scanHosts([]), []);
});

test('isPrivateIPv4', () => {
  for (const ip of ['10.1.2.3', '172.16.0.1', '172.31.255.1', '192.168.0.1']) assert.ok(isPrivateIPv4(ip), ip);
  for (const ip of ['172.32.0.1', '8.8.8.8', '127.0.0.1', '169.254.1.1', 'fe80::1', '1.2.3']) assert.ok(!isPrivateIPv4(ip), ip);
});

test('keyMessage matches netauth.KeyMessage', () => {
  assert.equal(new TextDecoder().decode(keyMessage('srv', 'n0')), 'agenttik-key-login-v1\nsrv\nn0');
});

test('base64url has no padding', () => {
  assert.equal(base64url(new Uint8Array([251, 255])), '-_8');
  assert.equal(base64url(new Uint8Array(32)).length, 43);
});

test('pool caps concurrency and visits every item', async () => {
  let live = 0;
  let peak = 0;
  const seen = [];
  await pool([1, 2, 3, 4, 5, 6, 7], 3, async (n) => {
    peak = Math.max(peak, ++live);
    await new Promise((r) => setTimeout(r, 5));
    seen.push(n);
    live--;
  });
  assert.equal(peak, 3);
  assert.deepEqual(seen.sort(), [1, 2, 3, 4, 5, 6, 7]);
});

test('isLocalOrigin matches the desktop', () => {
  for (const o of ['http://192.168.1.5:7717', 'http://10.0.0.2:7717', 'http://127.0.0.1:7717', 'http://169.254.1.1:7717',
    'http://[fd00::1]:7717', 'http://studio:7717', 'http://studio.local:7717', 'http://box.home.arpa']) {
    assert.ok(isLocalOrigin(o), o);
  }
  for (const o of ['https://a.trycloudflare.com', 'http://203.0.113.9:7717', 'http://[2001:db8::1]:7717', 'nope']) {
    assert.ok(!isLocalOrigin(o), o);
  }
});

test('candidates put local origins first, then the most recent', () => {
  const server = { origin: 'https://a.trycloudflare.com', origins: ['https://a.trycloudflare.com', 'http://203.0.113.9:7717', 'http://192.168.1.5:7717'] };
  assert.deepEqual(candidates(server), ['http://192.168.1.5:7717', 'https://a.trycloudflare.com', 'http://203.0.113.9:7717']);
});

test('remember keeps one entry per server with all its origins', () => {
  const home = 'http://192.168.1.5:7717';
  const away = 'https://a.trycloudflare.com';
  let list = remember([], { origin: home, id: 'lab', name: 'Lab' }, [away, 'bad/path', home]);
  assert.deepEqual(list, [{ origin: home, id: 'lab', name: 'Lab', origins: [home, away] }]);
  list = remember([{ origin: 'http://other:7717', id: 'other' }, ...list], { origin: away, id: 'lab', name: 'Lab' });
  assert.deepEqual(list.map((s) => [s.id, s.origin, s.origins]), [['lab', away, [away, home]], ['other', 'http://other:7717', undefined]]);
  // Another server now at a saved origin replaces the one last seen there.
  list = remember(list, { origin: 'http://other:7717', id: 'new' });
  assert.deepEqual(list.map((s) => s.id), ['new', 'lab']);
  const many = Array.from({ length: 12 }, (_, i) => `http://10.0.0.${i + 1}:7717`);
  assert.equal(remember([], { origin: home, id: 'lab' }, many)[0].origins.length, MAX_ORIGINS);
});

test('locate picks the best origin that answers, quickly', async () => {
  const wait = (ms) => new Promise((r) => setTimeout(r, ms));
  const servers = { dead: -1, fast: 0, slow: 60, silent: 1000 };
  const started = [];
  const check = async (o) => {
    started.push(o);
    if (servers[o] < 0) throw new Error(`down ${o}`);
    await wait(servers[o]);
    return { id: o };
  };
  const opts = { stagger: 30, grace: 100 };
  const run = async (...origins) => {
    started.length = 0;
    const t = Date.now();
    const { origin } = await locate(origins, check, opts);
    return [origin, Date.now() - t];
  };
  assert.equal((await run('dead', 'fast'))[0], 'fast');
  // The best answering first does not start the others.
  const [best, took] = await run('fast', 'silent');
  assert.equal(best, 'fast');
  assert.ok(took < 30, `${took} ms`);
  assert.deepEqual(started, ['fast']);
  // A better origin that answers within the grace wins.
  assert.equal((await run('slow', 'fast'))[0], 'slow');
  // One that never answers costs the grace, no more.
  const [fallback, waited] = await run('silent', 'fast');
  assert.equal(fallback, 'fast');
  assert.ok(waited < 30 + 100 + 50, `${waited} ms`);
  await assert.rejects(locate(['dead', 'dead2'], async (o) => { throw new Error(o); }, opts), /dead$/);
});
